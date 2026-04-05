export default function Introduction() {
  return (
    <>
      <h1>rpc-proxy</h1>
      <p className="subtitle">
        High-performance RPC reverse proxy for Tezos and EVM chains
      </p>

      <p>
        rpc-proxy sits in front of your blockchain nodes and provides a single, reliable
        endpoint for clients. It load-balances across backends, caches responses that
        auto-invalidate on new blocks, enforces per-IP rate limits, and routes historical
        queries to archive nodes automatically.
      </p>

      <h2 id="features">Key Features</h2>
      <ul>
        <li>
          <strong>Multi-chain</strong> — serves both Tezos (REST API) and Etherlink/EVM
          (JSON-RPC) from a single process
        </li>
        <li>
          <strong>Load balancing</strong> — round-robin across healthy nodes at the
          highest head level, with continuous health tracking
        </li>
        <li>
          <strong>Archive-aware routing</strong> — historical data requests are
          automatically routed to nodes marked as archive
        </li>
        <li>
          <strong>Generation-based caching</strong> — in-memory cache that invalidates
          when a new block arrives, with singleflight deduplication
        </li>
        <li>
          <strong>Tiered rate limiting</strong> — six cost tiers from high-throughput
          reads (300/s) down to debug traces (1/s), all per-IP
        </li>
        <li>
          <strong>Streaming</strong> — SSE pass-through for Tezos monitor endpoints with
          per-IP and global concurrency limits
        </li>
        <li>
          <strong>Fallback nodes</strong> — optional external fallback URLs tried when
          all primary nodes are down
        </li>
        <li>
          <strong>Hot config reload</strong> — rate limits, cache size, and stream limits
          update on SIGHUP or automatically on file change
        </li>
        <li>
          <strong>Zero-downtime upgrades</strong> — tableflip passes the listening socket
          to the new binary on SIGUSR2
        </li>
        <li>
          <strong>systemd integration</strong> — sends sd_notify READY=1 for
          Type=notify services
        </li>
      </ul>

      <h2 id="architecture">Architecture</h2>
      <p>
        Requests arrive at <code>/tezos/&lt;network&gt;/...</code> or{' '}
        <code>/etherlink/&lt;network&gt;/...</code> and flow through the filter (block
        dangerous endpoints, classify cost tier), rate limiter, cache, load balancer, and
        finally to an upstream node. Each network has its own balancer, cache, and block
        tracker running independently.
      </p>

      <h2 id="quick-start">Quick Start</h2>
      <p>
        See the <a href="/getting-started">Getting Started</a> page for build and run
        instructions, or jump to <a href="/configuration">Configuration</a> for the full
        config reference.
      </p>
    </>
  )
}
