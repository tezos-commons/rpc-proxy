package tracker

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tezos-commons/rpc-proxy/log"
	"github.com/valyala/fasthttp"
)

// evmBlockResponse holds the fields we need from eth_getBlockByNumber.
type evmBlockResponse struct {
	Number string `json:"number"`
	Hash   string `json:"hash"`
}

type evmJSONRPCResponse struct {
	Result json.RawMessage `json:"result"`
}

// EVMTracker monitors head block numbers on EVM nodes via eth_getBlockByNumber polling.
type EVMTracker struct {
	network      string
	nodes        []*NodeStatus
	recentBlocks *RecentBlocks
	onHead       func(int64) // called when a new head is observed
	logger       *log.Logger
	client       *fasthttp.Client
	interval     time.Duration
}

func NewEVMTracker(network string, nodes []*NodeStatus, recentBlocks *RecentBlocks, onHead func(int64), logger *log.Logger) *EVMTracker {
	return &EVMTracker{
		network:      network,
		nodes:        nodes,
		recentBlocks: recentBlocks,
		onHead:       onHead,
		logger:       logger,
		client: &fasthttp.Client{
			MaxResponseBodySize: 10 * 1024 * 1024, // 10MB
		},
		interval: 2 * time.Second,
	}
}

// LoadRecent fetches the last N blocks from the first responding node and
// populates the RecentBlocks store. Called once at startup.
func (e *EVMTracker) LoadRecent() {
	for _, node := range e.nodes {
		if err := e.loadRecentFrom(node); err != nil {
			e.logger.Warn(fmt.Sprintf("%s Load recent blocks failed: %s",
				log.Tag3(node.Name, "etherlink", e.network), log.Err(err)))
			continue
		}
		return // success
	}
	e.logger.Warn(fmt.Sprintf("%s Could not load recent blocks from any node",
		log.Tag2("etherlink", e.network)))
}

func (e *EVMTracker) loadRecentFrom(node *NodeStatus) error {
	// First get current head to know the range
	headNum, _, err := e.fetchBlock(node, "latest")
	if err != nil {
		return fmt.Errorf("fetch head: %w", err)
	}

	window := e.recentBlocks.Window()
	startBlock := headNum - window + 1
	if startBlock < 0 {
		startBlock = 0
	}

	// Batch fetch in groups of 100
	const batchSize = 100
	for from := startBlock; from <= headNum; from += batchSize {
		to := from + batchSize - 1
		if to > headNum {
			to = headNum
		}
		if err := e.fetchBlockBatch(node, from, to); err != nil {
			return fmt.Errorf("fetch batch %d-%d: %w", from, to, err)
		}
	}

	e.logger.Info(fmt.Sprintf("%s Loaded %d recent blocks, head %d",
		log.Tag3(node.Name, "etherlink", e.network), headNum-startBlock+1, headNum))
	return nil
}

func (e *EVMTracker) fetchBlockBatch(node *NodeStatus, from, to int64) error {
	// Build JSON-RPC batch
	type rpcReq struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
		Params  [2]any `json:"params"`
		ID      int64  `json:"id"`
	}

	count := to - from + 1
	batch := make([]rpcReq, count)
	for i := int64(0); i < count; i++ {
		batch[i] = rpcReq{
			JSONRPC: "2.0",
			Method:  "eth_getBlockByNumber",
			Params:  [2]any{fmt.Sprintf("0x%x", from+i), false},
			ID:      from + i,
		}
	}

	body, err := json.Marshal(batch)
	if err != nil {
		return err
	}

	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(node.URL)
	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.SetContentType("application/json")
	req.SetBody(body)

	if err := e.client.DoTimeout(req, resp, 30*time.Second); err != nil {
		return err
	}

	var results []evmJSONRPCResponse
	if err := json.Unmarshal(resp.Body(), &results); err != nil {
		return err
	}

	for _, r := range results {
		var block evmBlockResponse
		if err := json.Unmarshal(r.Result, &block); err != nil {
			continue
		}
		num, err := strconv.ParseInt(strings.TrimPrefix(block.Number, "0x"), 16, 64)
		if err != nil {
			continue
		}
		e.recentBlocks.Add(num, block.Hash)
	}

	return nil
}

func (e *EVMTracker) Start(ctx context.Context) {
	for _, node := range e.nodes {
		go e.poll(ctx, node)
	}
}

func (e *EVMTracker) poll(ctx context.Context, node *NodeStatus) {
	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()

	// Poll immediately on start
	e.fetchHead(node)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.fetchHead(node)
		}
	}
}

func (e *EVMTracker) fetchHead(node *NodeStatus) {
	head, hash, err := e.fetchBlock(node, "latest")
	if err != nil {
		node.SetUnhealthy()
		e.logger.Warn(fmt.Sprintf("%s Poll failed: %s",
			log.Tag3(node.Name, "etherlink", e.network), log.Err(err)))
		return
	}

	prevHead, _ := node.GetHead()
	node.Update(head)
	e.recentBlocks.Add(head, hash)

	if head != prevHead {
		if e.onHead != nil {
			e.onHead(head)
		}
		e.logger.Info(fmt.Sprintf("%s Head %d",
			log.Tag3(node.Name, "etherlink", e.network), head))
	}
}

var latestBlockBody = []byte(`{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["latest",false],"id":1}`)

// fetchBlock calls eth_getBlockByNumber and returns the block number and hash.
func (e *EVMTracker) fetchBlock(node *NodeStatus, blockTag string) (int64, string, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(node.URL)
	req.Header.SetMethod(fasthttp.MethodPost)
	req.Header.SetContentType("application/json")
	if blockTag == "latest" {
		req.SetBody(latestBlockBody)
	} else {
		req.SetBodyString(fmt.Sprintf(
			`{"jsonrpc":"2.0","method":"eth_getBlockByNumber","params":["%s",false],"id":1}`,
			blockTag,
		))
	}

	if err := e.client.DoTimeout(req, resp, 5*time.Second); err != nil {
		return 0, "", err
	}

	var rpcResp evmJSONRPCResponse
	if err := json.Unmarshal(resp.Body(), &rpcResp); err != nil {
		return 0, "", fmt.Errorf("bad json: %w", err)
	}

	var block evmBlockResponse
	if err := json.Unmarshal(rpcResp.Result, &block); err != nil {
		return 0, "", fmt.Errorf("bad block: %w", err)
	}

	hexStr := strings.TrimPrefix(block.Number, "0x")
	num, err := strconv.ParseInt(hexStr, 16, 64)
	if err != nil {
		return 0, "", fmt.Errorf("bad block number %q: %w", block.Number, err)
	}

	return num, block.Hash, nil
}
