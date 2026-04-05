import CodeBlock from '../components/CodeBlock'
import Callout from '../components/Callout'

export default function Deployment() {
  return (
    <>
      <h1>Deployment</h1>
      <p className="subtitle">
        systemd, zero-downtime upgrades, and config reload
      </p>

      <h2 id="systemd">systemd Service</h2>
      <p>
        rpc-proxy integrates with systemd's <code>Type=notify</code> protocol. It sends{' '}
        <code>READY=1</code> once the server is listening and ready to accept requests.
      </p>
      <CodeBlock title="/etc/systemd/system/rpc-proxy.service">
{`[Unit]
Description=RPC Proxy
After=network.target

[Service]
Type=notify
ExecStart=/usr/local/bin/rpc-proxy serve -c /etc/rpc-proxy/config.yaml
ExecReload=/bin/kill -USR2 $MAINPID
Restart=on-failure
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target`}
      </CodeBlock>

      <CodeBlock>
{`sudo systemctl daemon-reload
sudo systemctl enable --now rpc-proxy`}
      </CodeBlock>

      <h2 id="zero-downtime">Zero-Downtime Upgrades</h2>
      <p>
        rpc-proxy uses <a href="https://github.com/cloudflare/tableflip" target="_blank" rel="noopener noreferrer">tableflip</a>{' '}
        for graceful binary upgrades. On <code>SIGUSR2</code>:
      </p>
      <ol>
        <li>The running process forks and passes the listening socket fd to the new binary</li>
        <li>The new process starts, binds to the inherited socket, and signals ready</li>
        <li>The old process stops accepting new connections</li>
        <li>In-flight requests on the old process drain (up to 30s timeout)</li>
        <li>The old process exits</li>
      </ol>
      <p>
        During this entire process, there is <strong>no downtime</strong> — the listening
        socket is never closed.
      </p>

      <CodeBlock>
{`# Deploy a new binary
sudo cp rpc-proxy-new /usr/local/bin/rpc-proxy

# Trigger zero-downtime upgrade
sudo systemctl reload rpc-proxy`}
      </CodeBlock>

      <Callout variant="tip">
        The <code>ExecReload</code> directive sends <code>SIGUSR2</code> to the main PID,
        which triggers tableflip. This is different from a typical <code>SIGHUP</code>{' '}
        reload — it replaces the entire binary.
      </Callout>

      <h2 id="config-reload">Config Reload</h2>
      <p>
        Hot-reloadable config fields can be updated without restarting or upgrading the
        binary. There are two trigger mechanisms:
      </p>
      <ul>
        <li>
          <strong>SIGHUP</strong> — <code>kill -HUP $(pidof rpc-proxy)</code>
        </li>
        <li>
          <strong>File polling</strong> — the config file's modification time is checked
          every 30 seconds. If it changed, the config is reloaded automatically.
        </li>
      </ul>

      <h3 id="reloadable-fields">Hot-Reloadable Fields</h3>
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
          <tr><td><code>server.port</code></td><td>No — logged as warning</td></tr>
          <tr><td><code>chains.*</code></td><td>No — logged as warning</td></tr>
        </tbody>
      </table>

      <Callout variant="warning">
        Changes to <code>server.port</code> or <code>chains</code> are detected and
        logged as warnings, but they require a full restart (or binary upgrade via
        SIGUSR2) to take effect.
      </Callout>

      <h2 id="signals">Signal Summary</h2>
      <table>
        <thead>
          <tr>
            <th>Signal</th>
            <th>Action</th>
          </tr>
        </thead>
        <tbody>
          <tr><td><code>SIGUSR2</code></td><td>Zero-downtime binary upgrade (tableflip)</td></tr>
          <tr><td><code>SIGHUP</code></td><td>Reload config file (hot-reloadable fields only)</td></tr>
          <tr><td><code>SIGINT</code></td><td>Graceful shutdown</td></tr>
          <tr><td><code>SIGTERM</code></td><td>Graceful shutdown</td></tr>
        </tbody>
      </table>

      <h2 id="recommendations">Recommendations</h2>
      <ul>
        <li>
          Set <code>LimitNOFILE=65536</code> to handle many concurrent connections
        </li>
        <li>
          Place behind a reverse proxy (nginx, Caddy) that sets <code>X-Real-IP</code>{' '}
          for accurate rate limiting
        </li>
        <li>
          Configure fallback nodes per network for resilience
        </li>
        <li>
          Monitor the <code>/ready</code> endpoint from your load balancer
        </li>
        <li>
          Watch logs for request rates, error percentages, and cache hit ratios
        </li>
      </ul>
    </>
  )
}
