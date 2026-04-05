import CodeBlock from '../components/CodeBlock'
import ConfigTable from '../components/ConfigTable'
import Callout from '../components/Callout'

export default function Configuration() {
  return (
    <>
      <h1>Configuration</h1>
      <p className="subtitle">Full reference for config.yaml</p>

      <h2 id="full-example">Full Example</h2>
      <CodeBlock title="config.yaml">
{`server:
  port: 8080
  max_streams: 256

rate_limits:
  disabled: false
  default: 300
  expensive: 20
  injection: 10
  script: 5
  streaming: 5
  debug: 1

cache_max_entries: 10000

chains:
  tezos:
    networks:
      mainnet:
        nodes:
          - name: node-1
            url: "http://10.0.0.1:8732"
          - name: node-2
            url: "http://10.0.0.2:8732"
            archive: true
        fallbacks:
          - "https://mainnet.api.tez.ie"
      ghostnet:
        nodes:
          - name: node-3
            url: "http://10.0.0.3:8732"
  etherlink:
    networks:
      mainnet:
        nodes:
          - name: node-4
            url: "http://10.0.0.4:8545"
          - name: node-5
            url: "http://10.0.0.5:8545"
            archive: true`}
      </CodeBlock>

      <h2 id="server">server</h2>
      <ConfigTable
        rows={[
          { field: 'port', type: 'int', def: '8080', desc: 'Listen port (1–65535)' },
          {
            field: 'max_streams',
            type: 'int',
            def: '256',
            desc: 'Global max concurrent streaming connections',
          },
        ]}
      />

      <h2 id="rate-limits">rate_limits</h2>
      <p>
        Per-IP token-bucket rate limits in requests per second. See{' '}
        <a href="/rate-limiting">Rate Limiting</a> for details on each tier.
      </p>
      <ConfigTable
        rows={[
          { field: 'disabled', type: 'bool', def: 'false', desc: 'Disable all rate limiting' },
          { field: 'default', type: 'int', def: '300', desc: 'Read-only chain data' },
          { field: 'expensive', type: 'int', def: '20', desc: 'eth_call, eth_getLogs, big_maps, etc.' },
          { field: 'injection', type: 'int', def: '10', desc: 'eth_sendRawTransaction, /injection/operation' },
          { field: 'script', type: 'int', def: '5', desc: 'run_code, trace_code, typecheck_*' },
          { field: 'streaming', type: 'int', def: '5', desc: '/monitor/*, mempool monitor' },
          { field: 'debug', type: 'int', def: '1', desc: 'debug_trace*, tez_replayBlock' },
        ]}
      />
      <Callout variant="info">
        All rate limit values must be &ge; 1 when rate limiting is enabled.
      </Callout>

      <h2 id="cache">cache_max_entries</h2>
      <ConfigTable
        rows={[
          {
            field: 'cache_max_entries',
            type: 'int',
            def: '10000',
            desc: 'Max cached responses per network. Must be >= 1.',
          },
        ]}
      />

      <h2 id="chains">chains</h2>
      <p>
        Top-level chain families: <code>tezos</code> and <code>etherlink</code>. Each
        contains a <code>networks</code> map keyed by network name (e.g., mainnet,
        ghostnet, testnet).
      </p>

      <h3 id="nodes">nodes</h3>
      <ConfigTable
        rows={[
          { field: 'name', type: 'string', def: '—', desc: 'Unique node identifier within the network' },
          { field: 'url', type: 'string', def: '—', desc: 'Node RPC URL (http or https)' },
          {
            field: 'archive',
            type: 'bool',
            def: 'false',
            desc: 'Mark as archive node for historical queries',
          },
        ]}
      />

      <h3 id="fallbacks">fallbacks</h3>
      <p>
        Optional list of external RPC URLs per network. Tried in order when all primary
        nodes are unavailable, before returning 502.
      </p>

      <h2 id="validation">Validation Rules</h2>
      <ul>
        <li>At least one network must be configured across all chains</li>
        <li>Each network needs at least one node</li>
        <li>Node URLs must be valid http/https with a host</li>
        <li>No duplicate node names within a network</li>
        <li>Port must be 1–65535</li>
        <li>Rate limit values must all be &ge; 1</li>
        <li>cache_max_entries must be &ge; 1</li>
        <li>max_streams must be &ge; 1</li>
      </ul>

      <h2 id="hot-reload">Hot Reload</h2>
      <p>
        Some fields can be changed without restarting the process. Changes are picked up
        via <code>SIGHUP</code> or automatically when the config file's modification time
        changes (polled every 30 seconds).
      </p>
      <table>
        <thead>
          <tr>
            <th>Field</th>
            <th>Hot-reloadable</th>
          </tr>
        </thead>
        <tbody>
          <tr><td><code>rate_limits.*</code></td><td>Yes</td></tr>
          <tr><td><code>cache_max_entries</code></td><td>Yes</td></tr>
          <tr><td><code>server.max_streams</code></td><td>Yes</td></tr>
          <tr><td><code>server.port</code></td><td>No (restart required)</td></tr>
          <tr><td><code>chains.*</code></td><td>No (restart required)</td></tr>
        </tbody>
      </table>
    </>
  )
}
