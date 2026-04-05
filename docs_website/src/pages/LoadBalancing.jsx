import Callout from '../components/Callout'

export default function LoadBalancing() {
  return (
    <>
      <h1>Load Balancing</h1>
      <p className="subtitle">
        Round-robin across healthy nodes with archive-aware routing
      </p>

      <h2 id="algorithm">Algorithm</h2>
      <p>When a request needs to be forwarded upstream, the balancer:</p>
      <ol>
        <li>
          <strong>Selects candidates</strong> — picks all nodes reporting the highest
          head level (block height)
        </li>
        <li>
          <strong>Filters for archive</strong> — if the request needs historical data,
          only archive-flagged nodes are kept
        </li>
        <li>
          <strong>Round-robins</strong> — among the remaining candidates using an atomic
          counter
        </li>
      </ol>
      <p>
        If no healthy nodes are available (or no archive nodes for a historical request),
        the balancer returns <code>nil</code> and the proxy tries fallback URLs.
      </p>

      <h2 id="health-tracking">Health Tracking</h2>
      <p>
        Each node's health is tracked by a dedicated <strong>tracker</strong> goroutine
        that monitors the chain head:
      </p>
      <ul>
        <li>
          <strong>Tezos</strong> — streams <code>/monitor/heads/main</code> (SSE). On
          disconnect, reconnects with exponential backoff (1s → 30s). Backoff resets if
          the stream lasted more than 5 seconds.
        </li>
        <li>
          <strong>EVM</strong> — polls <code>eth_getBlockByNumber("latest")</code> every
          2 seconds
        </li>
      </ul>
      <p>
        A node is considered <strong>healthy</strong> if it has reported a head and its
        last update was within <strong>60 seconds</strong>. Nodes that go stale are
        excluded from rotation until they recover.
      </p>

      <h2 id="archive-routing">Archive-Aware Routing</h2>
      <p>
        The proxy automatically determines whether a request needs historical data and
        routes it to an archive node when necessary.
      </p>

      <h3 id="tezos-archive">Tezos</h3>
      <p>
        The block reference in the URL path is parsed (e.g.,{' '}
        <code>/blocks/&lt;blockID&gt;</code>):
      </p>
      <ul>
        <li>
          <code>head</code>, <code>head~N</code>, <code>head-N</code> → any node
        </li>
        <li>
          Numeric level → archive if <code>level &lt; head - window</code>
        </li>
        <li>Block hash → archive if not in the recent blocks set</li>
        <li>Unknown or genesis → archive (conservative)</li>
      </ul>

      <h3 id="evm-archive">EVM</h3>
      <p>
        The block tag parameter is inspected based on the JSON-RPC method:
      </p>
      <ul>
        <li>
          <code>"latest"</code>, <code>"pending"</code>, <code>"safe"</code>,{' '}
          <code>"finalized"</code> → any node
        </li>
        <li><code>"earliest"</code> → archive</li>
        <li>
          Hex block number → archive if{' '}
          <code>number &lt; head - window</code>
        </li>
        <li>
          <code>eth_getLogs</code> — checks <code>fromBlock</code>/
          <code>toBlock</code> in the filter object
        </li>
      </ul>

      <Callout variant="warning">
        If no archive nodes are configured for a network, the proxy logs a warning at
        startup. Historical requests will fail with 502 since there are no eligible
        nodes.
      </Callout>

      <h2 id="recent-blocks">Recent Blocks</h2>
      <p>
        The proxy maintains a sliding window of the last <strong>500 blocks</strong>{' '}
        (level → hash mapping) for each network. This is used to determine whether a
        block hash refers to recent or historical data.
      </p>
      <p>
        The recent blocks store handles chain reorganizations by removing all entries at
        or above the reorg level.
      </p>

      <h2 id="head-generation">Head Generation</h2>
      <p>
        Every time any node in a network reports a new head that advances the highest
        known level, the balancer increments an atomic <strong>generation counter</strong>
        . This counter is read by the cache to determine whether entries are stale — it's
        the mechanism that couples load balancing to cache invalidation.
      </p>

      <h2 id="fallbacks">Fallback Nodes</h2>
      <p>
        Each network can configure optional <code>fallbacks</code> — external RPC URLs
        that are tried when all primary nodes are down. Fallbacks are tried in order; the
        first successful response is returned. If all fallbacks also fail, the proxy
        returns 502 (Tezos) or a JSON-RPC error (EVM).
      </p>
    </>
  )
}
