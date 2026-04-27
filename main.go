package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cloudflare/tableflip"
	"github.com/spf13/cobra"
	"github.com/valyala/fasthttp"

	"github.com/tezos-commons/rpc-proxy/balancer"
	"github.com/tezos-commons/rpc-proxy/cache"
	"github.com/tezos-commons/rpc-proxy/config"
	"github.com/tezos-commons/rpc-proxy/filter"
	"github.com/tezos-commons/rpc-proxy/log"
	"github.com/tezos-commons/rpc-proxy/metrics"
	"github.com/tezos-commons/rpc-proxy/proxy"
	"github.com/tezos-commons/rpc-proxy/ratelimit"
	"github.com/tezos-commons/rpc-proxy/tracker"
)

func main() {
	var configPath string

	rootCmd := &cobra.Command{
		Use:   "rpc-proxy",
		Short: "High-performance RPC proxy for Tezos and EVM chains",
	}

	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the RPC proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServe(configPath)
		},
	}

	serveCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "path to config file")
	rootCmd.AddCommand(serveCmd)

	rootCmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runServe(configPath)
	}
	rootCmd.Flags().StringVarP(&configPath, "config", "c", "config.yaml", "path to config file")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runServe(configPath string) error {
	logger := log.New()
	logger.Info("rpc-proxy starting")

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	maxEntries := int64(cfg.CacheMaxEntries)

	// Build network sets (balancer + cache + recent blocks) and start trackers
	tezosNetworks := make(map[string]*proxy.NetworkSet)
	etherlinkNetworks := make(map[string]*proxy.NetworkSet)

	// Tezos networks
	for network, netCfg := range cfg.Chains.Tezos.Networks {
		nodes := make([]*tracker.NodeStatus, len(netCfg.Nodes))
		hasArchive := false
		for i, n := range netCfg.Nodes {
			nodes[i] = tracker.NewNodeStatus(n.Name, n.URL, n.Archive)
			if n.Archive {
				hasArchive = true
			}
		}
		if !hasArchive {
			logger.Warn(fmt.Sprintf("%s No archive nodes configured — historical requests will fail",
				log.Tag2("tezos", network)))
		}

		bal := balancer.New(nodes)
		nc := cache.New(bal.HeadGenerationPtr(), maxEntries)
		go nc.StartSweep(ctx)

		rb := tracker.NewRecentBlocks(tracker.DefaultBlockWindow)

		tezosNetworks[network] = &proxy.NetworkSet{
			Balancer:     bal,
			Cache:        nc,
			RecentBlocks: rb,
			Fallbacks:    netCfg.Fallbacks,
		}

		t := tracker.NewTezosTracker(network, nodes, rb, bal.NotifyHead, logger)
		t.LoadRecent()
		t.Start(ctx)

		logger.Info(fmt.Sprintf("%s Started with %d nodes, %d fallbacks",
			log.Tag2("tezos", network), len(nodes), len(netCfg.Fallbacks)))
	}

	// Etherlink networks
	for network, netCfg := range cfg.Chains.Etherlink.Networks {
		nodes := make([]*tracker.NodeStatus, len(netCfg.Nodes))
		hasArchive := false
		for i, n := range netCfg.Nodes {
			nodes[i] = tracker.NewNodeStatus(n.Name, n.URL, n.Archive)
			if n.Archive {
				hasArchive = true
			}
		}
		if !hasArchive {
			logger.Warn(fmt.Sprintf("%s No archive nodes configured — historical requests will fail",
				log.Tag2("etherlink", network)))
		}

		bal := balancer.New(nodes)
		nc := cache.New(bal.HeadGenerationPtr(), maxEntries)
		go nc.StartSweep(ctx)

		rb := tracker.NewRecentBlocks(tracker.DefaultBlockWindow)

		etherlinkNetworks[network] = &proxy.NetworkSet{
			Balancer:     bal,
			Cache:        nc,
			RecentBlocks: rb,
			Fallbacks:    netCfg.Fallbacks,
		}

		t := tracker.NewEVMTracker(network, nodes, rb, bal.NotifyHead, logger)
		t.LoadRecent()
		t.Start(ctx)

		logger.Info(fmt.Sprintf("%s Started with %d nodes, %d fallbacks",
			log.Tag2("etherlink", network), len(nodes), len(netCfg.Fallbacks)))
	}

	// Start metrics logger
	m := metrics.New(logger)
	go m.StartLogger(ctx)

	// Start per-IP rate limiter from config
	limiter := newLimiter(cfg, logger)
	go limiter.StartCleanup(ctx)

	// Build handler
	handler := proxy.NewHandler(tezosNetworks, etherlinkNetworks, m, limiter, int64(cfg.Server.MaxStreams), logger)

	// Start config reloader (SIGHUP + file mtime polling)
	reloadState := &reloadableState{
		limiter:           limiter,
		handler:           handler,
		tezosNetworks:     tezosNetworks,
		etherlinkNetworks: etherlinkNetworks,
		currentCfg:        cfg,
	}
	go watchConfig(ctx, configPath, reloadState, logger)

	// Start server
	server := &fasthttp.Server{
		Handler:            handler.HandleRequest,
		Name:               "rpc-proxy",
		MaxRequestBodySize: 10 * 1024 * 1024, // 10MB
		Concurrency:        256 * 1024,        // high limit — rate limiting and upstream clients handle backpressure
	}

	addr := fmt.Sprintf(":%d", cfg.Server.Port)

	// Tableflip: graceful upgrades via SIGUSR2.
	// On upgrade the old process passes the listening fd to the new binary,
	// keeps serving in-flight requests, then exits once drained.
	upg, err := tableflip.New(tableflip.Options{
		UpgradeTimeout: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("tableflip: %w", err)
	}
	defer upg.Stop()

	// Inherit or create the listening socket
	ln, err := upg.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer ln.Close()

	logger.Info(fmt.Sprintf("Listening on %s (cache: %d max entries)", addr, cfg.CacheMaxEntries))

	// Signal that init is done — the old process can now stop accepting
	if err := upg.Ready(); err != nil {
		return fmt.Errorf("ready: %w", err)
	}

	// Notify systemd that we're ready (tableflip doesn't do this itself)
	sdNotify()

	// Serve on the inherited/created listener
	go server.Serve(ln.(net.Listener))

	// Handle SIGUSR2 for zero-downtime upgrades
	sigUSR2 := make(chan os.Signal, 1)
	signal.Notify(sigUSR2, syscall.SIGUSR2)
	go func() {
		for range sigUSR2 {
			logger.Info("SIGUSR2 received, upgrading...")
			if err := upg.Upgrade(); err != nil {
				logger.Error(fmt.Sprintf("Upgrade failed: %s", log.Err(err)))
			}
		}
	}()

	// Wait for SIGINT/SIGTERM (shutdown) or tableflip exit (upgrade complete)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		logger.Info("Shutting down...")
	case <-upg.Exit():
		logger.Info("Upgrade complete, old process exiting...")
	}

	cancel()
	server.Shutdown()
	return nil
}

func newLimiter(cfg *config.Config, logger *log.Logger) *ratelimit.IPRateLimiter {
	rl := cfg.RateLimits
	var rates [filter.NumTiers]int
	rates[filter.TierDefault] = rl.Default
	rates[filter.TierExpensive] = rl.Expensive
	rates[filter.TierInjection] = rl.Injection
	rates[filter.TierScript] = rl.Script
	rates[filter.TierStreaming] = rl.Streaming
	rates[filter.TierDebug] = rl.Debug
	limiter := ratelimit.New(rates, rl.Disabled)

	logger.Info(fmt.Sprintf("%s Rate limits: default=%d expensive=%d injection=%d script=%d streaming=%d debug=%d",
		log.Tag("config"), rl.Default, rl.Expensive, rl.Injection, rl.Script, rl.Streaming, rl.Debug))

	return limiter
}

func rateLimitsFromConfig(cfg *config.Config) [filter.NumTiers]int {
	rl := cfg.RateLimits
	var rates [filter.NumTiers]int
	rates[filter.TierDefault] = rl.Default
	rates[filter.TierExpensive] = rl.Expensive
	rates[filter.TierInjection] = rl.Injection
	rates[filter.TierScript] = rl.Script
	rates[filter.TierStreaming] = rl.Streaming
	rates[filter.TierDebug] = rl.Debug
	return rates
}

// reloadableState holds references to components that can be hot-reconfigured.
type reloadableState struct {
	limiter           *ratelimit.IPRateLimiter
	handler           *proxy.Handler
	tezosNetworks     map[string]*proxy.NetworkSet
	etherlinkNetworks map[string]*proxy.NetworkSet
	currentCfg        *config.Config
}

// watchConfig watches for config file changes via SIGHUP and file mtime polling.
func watchConfig(ctx context.Context, configPath string, state *reloadableState, logger *log.Logger) {
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	var lastModTime time.Time
	if info, err := os.Stat(configPath); err == nil {
		lastModTime = info.ModTime()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-sighup:
			logger.Info(fmt.Sprintf("%s SIGHUP received, reloading config...", log.Tag("config")))
			reloadConfig(configPath, state, logger)
		case <-ticker.C:
			info, err := os.Stat(configPath)
			if err != nil {
				continue
			}
			if info.ModTime().After(lastModTime) {
				lastModTime = info.ModTime()
				logger.Info(fmt.Sprintf("%s Config file changed, reloading...", log.Tag("config")))
				reloadConfig(configPath, state, logger)
			}
		}
	}
}

func reloadConfig(configPath string, state *reloadableState, logger *log.Logger) {
	newCfg, err := config.Load(configPath)
	if err != nil {
		logger.Error(fmt.Sprintf("%s Reload failed (load): %s", log.Tag("config"), log.Err(err)))
		return
	}
	if err := newCfg.Validate(); err != nil {
		logger.Error(fmt.Sprintf("%s Reload failed (validate): %s", log.Tag("config"), log.Err(err)))
		return
	}

	old := state.currentCfg
	applied := 0

	// Rate limits
	oldRates := rateLimitsFromConfig(old)
	newRates := rateLimitsFromConfig(newCfg)
	if oldRates != newRates || old.RateLimits.Disabled != newCfg.RateLimits.Disabled {
		state.limiter.SetDisabled(newCfg.RateLimits.Disabled)
		if !newCfg.RateLimits.Disabled {
			state.limiter.UpdateRates(newRates)
		}
		rl := newCfg.RateLimits
		logger.Info(fmt.Sprintf("%s Rate limits updated: default=%d expensive=%d injection=%d script=%d streaming=%d debug=%d disabled=%v",
			log.Tag("config"), rl.Default, rl.Expensive, rl.Injection, rl.Script, rl.Streaming, rl.Debug, rl.Disabled))
		applied++
	}

	// Cache max entries
	if old.CacheMaxEntries != newCfg.CacheMaxEntries {
		newMax := int64(newCfg.CacheMaxEntries)
		for _, ns := range state.tezosNetworks {
			ns.Cache.SetMaxSize(newMax)
		}
		for _, ns := range state.etherlinkNetworks {
			ns.Cache.SetMaxSize(newMax)
		}
		logger.Info(fmt.Sprintf("%s Cache max entries updated: %d -> %d",
			log.Tag("config"), old.CacheMaxEntries, newCfg.CacheMaxEntries))
		applied++
	}

	// Max streams
	if old.Server.MaxStreams != newCfg.Server.MaxStreams {
		state.handler.SetMaxStreams(int64(newCfg.Server.MaxStreams))
		logger.Info(fmt.Sprintf("%s Max streams updated: %d -> %d",
			log.Tag("config"), old.Server.MaxStreams, newCfg.Server.MaxStreams))
		applied++
	}

	// Warn about fields that require restart
	if old.Server.Port != newCfg.Server.Port {
		logger.Warn(fmt.Sprintf("%s server.port changed (%d -> %d) — requires restart to take effect",
			log.Tag("config"), old.Server.Port, newCfg.Server.Port))
	}
	if chainsChanged(old, newCfg) {
		logger.Warn(fmt.Sprintf("%s chains configuration changed — requires restart to take effect",
			log.Tag("config")))
	}

	if applied == 0 {
		logger.Info(fmt.Sprintf("%s Config reloaded, no hot-reloadable changes detected", log.Tag("config")))
	}

	state.currentCfg = newCfg
}

// chainsChanged returns true if network/node topology changed (not hot-reloadable).
func chainsChanged(old, new *config.Config) bool {
	if len(old.Chains.Tezos.Networks) != len(new.Chains.Tezos.Networks) {
		return true
	}
	if len(old.Chains.Etherlink.Networks) != len(new.Chains.Etherlink.Networks) {
		return true
	}
	for name, oldNet := range old.Chains.Tezos.Networks {
		newNet, ok := new.Chains.Tezos.Networks[name]
		if !ok || !nodesEqual(oldNet.Nodes, newNet.Nodes) || !stringsEqual(oldNet.Fallbacks, newNet.Fallbacks) {
			return true
		}
	}
	for name, oldNet := range old.Chains.Etherlink.Networks {
		newNet, ok := new.Chains.Etherlink.Networks[name]
		if !ok || !nodesEqual(oldNet.Nodes, newNet.Nodes) || !stringsEqual(oldNet.Fallbacks, newNet.Fallbacks) {
			return true
		}
	}
	return false
}

func nodesEqual(a, b []config.NodeConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func stringsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sdNotify sends READY=1 to systemd's notify socket if present.
// No-op when NOTIFY_SOCKET is unset (e.g. running outside systemd).
func sdNotify() {
	sock := os.Getenv("NOTIFY_SOCKET")
	if sock == "" {
		return
	}
	conn, err := net.Dial("unixgram", sock)
	if err != nil {
		return
	}
	conn.Write([]byte("READY=1"))
	conn.Close()
}
