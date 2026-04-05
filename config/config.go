package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server          ServerConfig     `yaml:"server"`
	Chains          ChainsConfig     `yaml:"chains"`
	RateLimits      RateLimitsConfig `yaml:"rate_limits"`
	CacheMaxEntries int              `yaml:"cache_max_entries"`
}

type ServerConfig struct {
	Port       int `yaml:"port"`
	MaxStreams int `yaml:"max_streams"` // max concurrent streaming connections
}

type ChainsConfig struct {
	Tezos     ChainConfig `yaml:"tezos"`
	Etherlink ChainConfig `yaml:"etherlink"`
}

type ChainConfig struct {
	Networks map[string]NetworkConfig `yaml:"networks"`
}

type NetworkConfig struct {
	Nodes     []NodeConfig `yaml:"nodes"`
	Fallbacks []string     `yaml:"fallbacks"`
}

type NodeConfig struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	Archive bool   `yaml:"archive"`
}

// RateLimitsConfig defines per-IP per-second rate limits for each tier.
type RateLimitsConfig struct {
	Disabled  bool `yaml:"disabled"`  // disable all rate limiting
	Default   int  `yaml:"default"`   // read-only chain data
	Expensive int  `yaml:"expensive"` // eth_call, eth_getLogs, big_maps, raw context, preapply
	Injection int  `yaml:"injection"` // eth_sendRawTransaction, /injection/operation
	Script    int  `yaml:"script"`    // run_code, trace_code, typecheck_*, simulate_operation
	Streaming int  `yaml:"streaming"` // /monitor/*, mempool monitor
	Debug     int  `yaml:"debug"`     // debug_trace*, tez_replayBlock
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.CacheMaxEntries == 0 {
		cfg.CacheMaxEntries = 10000
	}
	if cfg.Server.MaxStreams == 0 {
		cfg.Server.MaxStreams = 256
	}
	applyRateLimitDefaults(&cfg.RateLimits)
	return &cfg, nil
}

// Validate checks the configuration for errors. Returns nil if valid.
func (c *Config) Validate() error {
	// Server
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be 1-65535, got %d", c.Server.Port)
	}
	if c.Server.MaxStreams < 1 {
		return fmt.Errorf("server.max_streams must be >= 1, got %d", c.Server.MaxStreams)
	}

	// Cache
	if c.CacheMaxEntries < 1 {
		return fmt.Errorf("cache_max_entries must be >= 1, got %d", c.CacheMaxEntries)
	}

	// Rate limits
	if err := validateRateLimits(&c.RateLimits); err != nil {
		return err
	}

	// At least one network across all chains
	totalNetworks := len(c.Chains.Tezos.Networks) + len(c.Chains.Etherlink.Networks)
	if totalNetworks == 0 {
		return fmt.Errorf("at least one network must be configured")
	}

	for network, netCfg := range c.Chains.Tezos.Networks {
		if err := validateNetwork("tezos", network, &netCfg); err != nil {
			return err
		}
	}
	for network, netCfg := range c.Chains.Etherlink.Networks {
		if err := validateNetwork("etherlink", network, &netCfg); err != nil {
			return err
		}
	}

	return nil
}

func validateRateLimits(rl *RateLimitsConfig) error {
	if rl.Disabled {
		return nil
	}
	checks := []struct {
		name string
		val  int
	}{
		{"default", rl.Default},
		{"expensive", rl.Expensive},
		{"injection", rl.Injection},
		{"script", rl.Script},
		{"streaming", rl.Streaming},
		{"debug", rl.Debug},
	}
	for _, c := range checks {
		if c.val < 1 {
			return fmt.Errorf("rate_limits.%s must be >= 1, got %d", c.name, c.val)
		}
	}
	return nil
}

func validateNetwork(chain, network string, netCfg *NetworkConfig) error {
	if len(netCfg.Nodes) == 0 {
		return fmt.Errorf("chains.%s.networks.%s: at least one node required", chain, network)
	}

	names := make(map[string]struct{}, len(netCfg.Nodes))
	for i, node := range netCfg.Nodes {
		if strings.TrimSpace(node.Name) == "" {
			return fmt.Errorf("chains.%s.networks.%s.nodes[%d]: name is required", chain, network, i)
		}
		if strings.TrimSpace(node.URL) == "" {
			return fmt.Errorf("chains.%s.networks.%s.nodes[%d] (%s): url is required", chain, network, i, node.Name)
		}
		u, err := url.Parse(node.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("chains.%s.networks.%s.nodes[%d] (%s): invalid url %q", chain, network, i, node.Name, node.URL)
		}
		if _, dup := names[node.Name]; dup {
			return fmt.Errorf("chains.%s.networks.%s: duplicate node name %q", chain, network, node.Name)
		}
		names[node.Name] = struct{}{}
	}

	for i, fb := range netCfg.Fallbacks {
		u, err := url.Parse(fb)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("chains.%s.networks.%s.fallbacks[%d]: invalid url %q", chain, network, i, fb)
		}
	}

	return nil
}

func applyRateLimitDefaults(rl *RateLimitsConfig) {
	if rl.Default == 0 {
		rl.Default = 300
	}
	if rl.Expensive == 0 {
		rl.Expensive = 20
	}
	if rl.Injection == 0 {
		rl.Injection = 10
	}
	if rl.Script == 0 {
		rl.Script = 5
	}
	if rl.Streaming == 0 {
		rl.Streaming = 5
	}
	if rl.Debug == 0 {
		rl.Debug = 1
	}
}
