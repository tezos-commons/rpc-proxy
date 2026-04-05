package tracker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/tezos-commons/rpc-proxy/log"
)

type tezosHeadResponse struct {
	Level int64  `json:"level"`
	Hash  string `json:"hash"`
}

// TezosTracker monitors head levels on Tezos nodes via /monitor/heads/main streaming.
type TezosTracker struct {
	network      string
	nodes        []*NodeStatus
	recentBlocks *RecentBlocks
	onHead       func(int64) // called when a new head is observed
	logger       *log.Logger
	client       *http.Client // streaming (no timeout)
	fetchClient  *http.Client // one-shot requests (with timeout)
}

func NewTezosTracker(network string, nodes []*NodeStatus, recentBlocks *RecentBlocks, onHead func(int64), logger *log.Logger) *TezosTracker {
	return &TezosTracker{
		network:      network,
		nodes:        nodes,
		recentBlocks: recentBlocks,
		onHead:       onHead,
		logger:       logger,
		client: &http.Client{
			Timeout: 0, // no timeout for streaming
		},
		fetchClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// LoadRecent fetches the last N block hashes from the first healthy node
// and populates the RecentBlocks store. Called once at startup.
func (t *TezosTracker) LoadRecent() {
	for _, node := range t.nodes {
		if err := t.loadRecentFrom(node); err != nil {
			t.logger.Warn(fmt.Sprintf("%s Load recent blocks failed: %s",
				log.Tag3(node.Name, "tezos", t.network), log.Err(err)))
			continue
		}
		return // success
	}
	t.logger.Warn(fmt.Sprintf("%s Could not load recent blocks from any node",
		log.Tag2("tezos", t.network)))
}

func (t *TezosTracker) loadRecentFrom(node *NodeStatus) error {
	// GET /chains/main/blocks?length=N returns [[hash0, hash1, ...]] in
	// descending order (head first).
	url := fmt.Sprintf("%s/chains/main/blocks?length=%d", node.URL, t.recentBlocks.Window())

	resp, err := t.fetchClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var result [][]string
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	if len(result) == 0 || len(result[0]) == 0 {
		return fmt.Errorf("empty block list")
	}

	hashes := result[0]

	// We need the head level to compute levels for each hash.
	// Fetch it from the first hash (the head block).
	headLevel, err := t.fetchBlockLevel(node, hashes[0])
	if err != nil {
		return fmt.Errorf("fetch head level: %w", err)
	}

	for i, hash := range hashes {
		t.recentBlocks.Add(headLevel-int64(i), hash)
	}

	t.logger.Info(fmt.Sprintf("%s Loaded %d recent blocks, head %d",
		log.Tag3(node.Name, "tezos", t.network), len(hashes), headLevel))
	return nil
}

func (t *TezosTracker) fetchBlockLevel(node *NodeStatus, hash string) (int64, error) {
	url := fmt.Sprintf("%s/chains/main/blocks/%s/header", node.URL, hash)
	resp, err := t.fetchClient.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var header struct {
		Level int64 `json:"level"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&header); err != nil {
		return 0, err
	}
	return header.Level, nil
}

func (t *TezosTracker) Start(ctx context.Context) {
	for _, node := range t.nodes {
		go t.monitor(ctx, node)
	}
}

func (t *TezosTracker) monitor(ctx context.Context, node *NodeStatus) {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		err := t.stream(ctx, node)
		if err != nil {
			node.SetUnhealthy()
			t.logger.Warn(fmt.Sprintf("%s Monitor disconnected: %s",
				log.Tag3(node.Name, "tezos", t.network), log.Err(err)))
		}

		// Reset backoff if the stream was alive long enough to have been useful
		if time.Since(start) > 5*time.Second {
			backoff = time.Second
		} else {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}

		backoffTimer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			backoffTimer.Stop()
			return
		case <-backoffTimer.C:
		}
	}
}

func (t *TezosTracker) stream(ctx context.Context, node *NodeStatus) error {
	url := node.URL + "/monitor/heads/main"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	// Increase buffer for potentially large JSON objects
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var head tezosHeadResponse
		if err := json.Unmarshal(line, &head); err != nil {
			// Some lines may not be valid JSON (e.g. empty), skip
			continue
		}

		if head.Level > 0 {
			node.Update(head.Level)
			t.recentBlocks.Add(head.Level, head.Hash)
			if t.onHead != nil {
				t.onHead(head.Level)
			}
			t.logger.Info(fmt.Sprintf("%s Head %d",
				log.Tag3(node.Name, "tezos", t.network), head.Level))
		}
	}

	return scanner.Err()
}
