import Callout from '../components/Callout'
import CodeBlock from '../components/CodeBlock'

export default function ApiReference() {
  return (
    <>
      <h1>API Reference</h1>
      <p className="subtitle">
        Blocked endpoints, error responses, and batch handling
      </p>

      <h2 id="blocked-tezos">Blocked Tezos Endpoints</h2>
      <p>
        The following Tezos endpoints are blocked and return <strong>403 Forbidden</strong>:
      </p>
      <table>
        <thead>
          <tr>
            <th>Path pattern</th>
            <th>Reason</th>
          </tr>
        </thead>
        <tbody>
          <tr><td><code>/network/*</code></td><td>P2P peer management</td></tr>
          <tr><td><code>/workers/*</code></td><td>Internal diagnostics</td></tr>
          <tr><td><code>/stats/*</code>, <code>/gc/*</code></td><td>Performance metrics</td></tr>
          <tr><td><code>/config</code> (GET)</td><td>Full config dump</td></tr>
          <tr><td><code>/config/logging</code> (PUT)</td><td>Node state mutation</td></tr>
          <tr><td><code>/private/*</code></td><td>Private injection</td></tr>
          <tr><td><code>/injection/block</code></td><td>Baker-only</td></tr>
          <tr><td><code>/injection/protocol</code></td><td>Baker-only</td></tr>
          <tr><td><code>/fetch_protocol/*</code></td><td>Triggers network activity</td></tr>
          <tr><td><code>/monitor/received_blocks/*</code></td><td>DoS vector</td></tr>
          <tr><td><code>/chains/*/active_peers_heads</code></td><td>Leaks peer info</td></tr>
          <tr><td><code>/invalid_blocks</code></td><td>Listing/deletion</td></tr>
          <tr><td><code>/mempool/ban_operation</code></td><td>Mutation</td></tr>
          <tr><td><code>/mempool/unban_*</code></td><td>Mutation</td></tr>
          <tr><td><code>/mempool/request_operations</code></td><td>Mutation</td></tr>
          <tr><td><code>/mempool/filter</code> (POST)</td><td>Mutation</td></tr>
          <tr><td><code>/context/seed</code> (POST)</td><td>Seed reveal</td></tr>
          <tr><td><code>/context/cache/contracts/all</code></td><td>Internal cache</td></tr>
          <tr><td><code>/context/cache/contracts/size</code></td><td>Internal cache</td></tr>
        </tbody>
      </table>

      <h2 id="blocked-evm">Blocked EVM Endpoints</h2>
      <h3 id="blocked-evm-rest">REST Paths</h3>
      <p>
        These Etherlink REST paths are blocked (403):
      </p>
      <ul>
        <li><code>/private</code> — private JSON-RPC endpoint</li>
        <li><code>/evm/*</code> — inter-node peer services</li>
        <li><code>/configuration</code> — config leak</li>
        <li><code>/mode</code> — infrastructure leak</li>
        <li><code>/metrics</code> — infrastructure leak</li>
      </ul>

      <h3 id="blocked-evm-methods">JSON-RPC Methods</h3>
      <p>
        The following EVM JSON-RPC methods are blocked and return a JSON-RPC error with
        message "method not allowed":
      </p>
      <ul>
        <li><code>produceBlock</code></li>
        <li><code>proposeNextBlockTimestamp</code></li>
        <li><code>producProposal</code></li>
        <li><code>executeSingleTransaction</code></li>
        <li><code>injectTransaction</code></li>
        <li><code>waitTransactionConfirmation</code></li>
        <li><code>injectTezlinkOperation</code></li>
        <li><code>lockBlockProduction</code></li>
        <li><code>unlockBlockProduction</code></li>
      </ul>

      <h2 id="tier-classification">Tier Classification</h2>
      <h3 id="tezos-tiers">Tezos</h3>
      <table>
        <thead>
          <tr>
            <th>Tier</th>
            <th>Endpoints</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td><code>streaming</code></td>
            <td><code>/monitor/*</code>, <code>/mempool/monitor_operations</code></td>
          </tr>
          <tr>
            <td><code>injection</code></td>
            <td><code>/injection/operation</code></td>
          </tr>
          <tr>
            <td><code>script</code></td>
            <td>
              <code>run_code</code>, <code>run_view</code>, <code>run_script_view</code>,{' '}
              <code>trace_code</code>, <code>typecheck_code</code>,{' '}
              <code>typecheck_data</code>, <code>simulate_operation</code>
            </td>
          </tr>
          <tr>
            <td><code>expensive</code></td>
            <td>
              <code>big_maps</code>, <code>context/raw/bytes</code>,{' '}
              <code>merkle_tree</code>, <code>preapply</code>, <code>sapling</code>,{' '}
              <code>list contracts</code>, <code>list delegates</code>
            </td>
          </tr>
          <tr>
            <td><code>default</code></td>
            <td>Everything else</td>
          </tr>
        </tbody>
      </table>

      <h3 id="evm-tiers">EVM</h3>
      <table>
        <thead>
          <tr>
            <th>Tier</th>
            <th>Methods</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td><code>debug</code></td>
            <td>
              <code>debug_traceTransaction</code>, <code>debug_traceCall</code>,{' '}
              <code>debug_traceBlockByNumber</code>, <code>debug_traceBlockByHash</code>,{' '}
              <code>tez_replayBlock</code>
            </td>
          </tr>
          <tr>
            <td><code>expensive</code></td>
            <td>
              <code>eth_call</code>, <code>eth_estimateGas</code>,{' '}
              <code>eth_getLogs</code>, <code>eth_createAccessList</code>
            </td>
          </tr>
          <tr>
            <td><code>injection</code></td>
            <td>
              <code>eth_sendRawTransaction</code>, <code>eth_sendRawTransactionSync</code>
            </td>
          </tr>
          <tr>
            <td><code>default</code></td>
            <td>Everything else</td>
          </tr>
        </tbody>
      </table>

      <Callout variant="warning">
        Requests with unparseable JSON-RPC bodies are classified as{' '}
        <strong>debug</strong> tier (1 req/s) to prevent rate limit bypass via
        garbage payloads.
      </Callout>

      <h2 id="error-responses">Error Responses</h2>
      <h3 id="tezos-errors">Tezos</h3>
      <table>
        <thead>
          <tr>
            <th>Status</th>
            <th>Body</th>
            <th>Cause</th>
          </tr>
        </thead>
        <tbody>
          <tr><td>403</td><td><code>forbidden</code></td><td>Blocked endpoint</td></tr>
          <tr><td>404</td><td><code>unknown tezos network</code></td><td>Unknown network name</td></tr>
          <tr><td>429</td><td><code>rate limit exceeded</code></td><td>Per-IP rate limit hit</td></tr>
          <tr><td>429</td><td><code>too many streaming connections from this IP</code></td><td>Per-IP stream limit (10)</td></tr>
          <tr><td>502</td><td><code>upstream error</code></td><td>All nodes + fallbacks failed</td></tr>
          <tr><td>503</td><td><code>too many streaming connections</code></td><td>Global stream limit hit</td></tr>
        </tbody>
      </table>

      <h3 id="evm-errors">EVM</h3>
      <p>
        EVM errors follow JSON-RPC format:
      </p>
      <CodeBlock>
{`{
  "jsonrpc": "2.0",
  "id": <caller-id>,
  "error": {
    "code": -32000,
    "message": "<error message>"
  }
}`}
      </CodeBlock>
      <table>
        <thead>
          <tr>
            <th>Message</th>
            <th>Cause</th>
          </tr>
        </thead>
        <tbody>
          <tr><td><code>method not allowed</code></td><td>Blocked JSON-RPC method</td></tr>
          <tr><td><code>rate limit exceeded</code></td><td>Per-IP rate limit hit</td></tr>
          <tr><td><code>no healthy node available</code></td><td>All nodes + fallbacks failed</td></tr>
          <tr><td><code>unknown etherlink network</code></td><td>Unknown network (HTTP 404)</td></tr>
        </tbody>
      </table>

      <h2 id="batch-requests">Batch Requests (EVM)</h2>
      <p>
        EVM supports JSON-RPC batch requests (JSON arrays). Each item in the batch is
        processed independently through the full pipeline:
      </p>
      <ul>
        <li>Rate limiting is applied per item</li>
        <li>Cache is checked per item</li>
        <li>Items are processed concurrently (up to 16 in parallel)</li>
        <li>One item's failure doesn't affect others</li>
      </ul>
      <table>
        <thead>
          <tr>
            <th>Condition</th>
            <th>Response</th>
          </tr>
        </thead>
        <tbody>
          <tr><td>Empty batch <code>[]</code></td><td><code>[]</code></td></tr>
          <tr><td>Invalid batch (not an array)</td><td>400 Bad Request</td></tr>
          <tr><td>Batch &gt; 100 items</td><td>400 Bad Request</td></tr>
        </tbody>
      </table>
    </>
  )
}
