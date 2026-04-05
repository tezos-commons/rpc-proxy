import CodeBlock from '../components/CodeBlock'
import Callout from '../components/Callout'

export default function Streaming() {
  return (
    <>
      <h1>Streaming</h1>
      <p className="subtitle">
        Server-Sent Events pass-through for Tezos monitor endpoints
      </p>

      <h2 id="overview">Overview</h2>
      <p>
        Tezos nodes expose streaming endpoints under <code>/monitor/</code> that use
        Server-Sent Events (SSE) to push updates in real time. rpc-proxy pipes these
        connections through to the upstream node without buffering.
      </p>

      <h2 id="streaming-endpoints">Streaming Endpoints</h2>
      <p>
        The following Tezos endpoints are treated as streaming:
      </p>
      <ul>
        <li><code>/monitor/heads/main</code></li>
        <li><code>/monitor/bootstrapped</code></li>
        <li><code>/monitor/active_chains</code></li>
        <li><code>/monitor/protocols</code></li>
        <li><code>/monitor/valid_blocks</code></li>
        <li><code>/mempool/monitor_operations</code></li>
      </ul>

      <h2 id="concurrency-limits">Concurrency Limits</h2>
      <p>
        Streaming connections are long-lived and consume resources on both the proxy and
        upstream node. Two concurrency limits protect against exhaustion:
      </p>
      <table>
        <thead>
          <tr>
            <th>Limit</th>
            <th>Default</th>
            <th>Configurable</th>
            <th>Error</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>Global</td>
            <td>256</td>
            <td>Yes (<code>server.max_streams</code>)</td>
            <td>503 Service Unavailable</td>
          </tr>
          <tr>
            <td>Per-IP</td>
            <td>10</td>
            <td>No (hardcoded)</td>
            <td>429 Too Many Requests</td>
          </tr>
        </tbody>
      </table>

      <Callout variant="info">
        The global <code>max_streams</code> limit is hot-reloadable. The per-IP limit of
        10 concurrent streams is hardcoded and cannot be changed via config.
      </Callout>

      <h2 id="implementation">Implementation Details</h2>
      <ul>
        <li>
          Streaming uses Go's standard <code>net/http</code> client (not fasthttp)
          because fasthttp doesn't support long-lived streaming responses
        </li>
        <li>No timeout is set on the HTTP client — streams are long-lived by design</li>
        <li>
          I/O uses pooled <strong>32KB buffers</strong> to minimize allocations
        </li>
        <li>
          The upstream connection is piped directly to the client via{' '}
          <code>SetBodyStreamWriter</code> — no intermediate buffering
        </li>
        <li>
          When the client disconnects, both the upstream connection and all tracking
          counters are cleaned up
        </li>
      </ul>

      <h2 id="rate-limiting">Rate Limiting</h2>
      <p>
        Streaming endpoints are classified as the <strong>streaming</strong> tier
        (default: 5 req/s). This limits how frequently a single IP can{' '}
        <em>establish</em> new streaming connections, independent of the concurrency
        limit on active streams.
      </p>

      <h2 id="example">Example</h2>
      <CodeBlock>
{`# Stream new block headers
curl -N http://localhost:8080/tezos/mainnet/monitor/heads/main

# Monitor mempool operations
curl -N http://localhost:8080/tezos/mainnet/chains/main/mempool/monitor_operations`}
      </CodeBlock>
    </>
  )
}
