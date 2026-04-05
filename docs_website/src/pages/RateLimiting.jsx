import CodeBlock from '../components/CodeBlock'
import Callout from '../components/Callout'

export default function RateLimiting() {
  return (
    <>
      <h1>Rate Limiting</h1>
      <p className="subtitle">Per-IP token bucket rate limiting with six cost tiers</p>

      <h2 id="tiers">Cost Tiers</h2>
      <p>
        Every request is classified into one of six tiers based on the RPC method or
        endpoint. Each tier has its own per-IP rate limit:
      </p>
      <table>
        <thead>
          <tr>
            <th>Tier</th>
            <th>Default rate</th>
            <th>Methods / Endpoints</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td><code>default</code></td>
            <td>300 req/s</td>
            <td>All read-only chain data not classified elsewhere</td>
          </tr>
          <tr>
            <td><code>expensive</code></td>
            <td>20 req/s</td>
            <td>
              <code>eth_call</code>, <code>eth_estimateGas</code>,{' '}
              <code>eth_getLogs</code>, big_maps, context/raw/bytes, merkle_tree,
              preapply, sapling, list contracts/delegates
            </td>
          </tr>
          <tr>
            <td><code>injection</code></td>
            <td>10 req/s</td>
            <td>
              <code>eth_sendRawTransaction</code>, <code>/injection/operation</code>
            </td>
          </tr>
          <tr>
            <td><code>script</code></td>
            <td>5 req/s</td>
            <td>
              <code>run_code</code>, <code>trace_code</code>, <code>typecheck_*</code>,{' '}
              <code>simulate_operation</code>
            </td>
          </tr>
          <tr>
            <td><code>streaming</code></td>
            <td>5 req/s</td>
            <td>
              <code>/monitor/*</code>, mempool monitor, SSE endpoints
            </td>
          </tr>
          <tr>
            <td><code>debug</code></td>
            <td>1 req/s</td>
            <td>
              <code>debug_trace*</code>, <code>tez_replayBlock</code>, unparseable
              requests
            </td>
          </tr>
        </tbody>
      </table>

      <Callout variant="warning">
        Unparseable JSON-RPC request bodies are assigned to the <strong>debug</strong>{' '}
        tier (1 req/s) to prevent bypassing rate limits with garbage payloads.
      </Callout>

      <h2 id="token-bucket">Token Bucket Algorithm</h2>
      <p>
        Each (IP, tier) pair gets an independent token bucket. Tokens refill continuously
        at the configured rate. A request is allowed if the bucket has &ge; 1 token.
      </p>
      <ul>
        <li><strong>Burst capacity</strong> = 1.5&times; rate (minimum 2)</li>
        <li>
          <strong>Fixed-point math</strong> — tokens are tracked at 1000&times; precision
          to avoid floating-point drift
        </li>
        <li>
          <strong>Lazy initialization</strong> — buckets are only created when an IP
          first sends a request to that tier
        </li>
      </ul>

      <h2 id="ip-tracking">IP Tracking</h2>
      <p>
        IP entries are stored in a sharded map (256 shards) for concurrent access. Each
        entry holds buckets for all six tiers.
      </p>
      <ul>
        <li>
          <strong>Max capacity</strong> — 100,000 tracked IPs. When exceeded, new IPs
          are denied (fail-closed) to prevent memory exhaustion
        </li>
        <li>
          <strong>Stale cleanup</strong> — IPs with no activity for 5 minutes are removed
          every 60 seconds
        </li>
      </ul>

      <h2 id="disabling">Disabling Rate Limits</h2>
      <p>
        Set <code>rate_limits.disabled: true</code> in your config to bypass all rate
        limiting. This is hot-reloadable — no restart needed.
      </p>
      <CodeBlock>
{`rate_limits:
  disabled: true`}
      </CodeBlock>

      <h2 id="error-responses">Error Responses</h2>
      <p>When rate-limited:</p>
      <ul>
        <li>
          <strong>Tezos</strong> — HTTP 429 with body <code>rate limit exceeded</code>
        </li>
        <li>
          <strong>EVM</strong> — JSON-RPC error:{' '}
          <code>{`{"jsonrpc":"2.0","error":{"code":-32000,"message":"rate limit exceeded"}}`}</code>
        </li>
      </ul>
    </>
  )
}
