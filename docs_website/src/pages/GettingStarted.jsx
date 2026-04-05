import CodeBlock from '../components/CodeBlock'
import Callout from '../components/Callout'

export default function GettingStarted() {
  return (
    <>
      <h1>Getting Started</h1>
      <p className="subtitle">Build, configure, and run rpc-proxy</p>

      <h2 id="requirements">Requirements</h2>
      <ul>
        <li>Go 1.25+</li>
        <li>One or more Tezos or EVM (Etherlink) RPC nodes</li>
      </ul>

      <h2 id="build">Build</h2>
      <CodeBlock>
{`go build -o rpc-proxy .`}
      </CodeBlock>

      <p>This produces a single static binary with no runtime dependencies.</p>

      <h2 id="run">Run</h2>
      <CodeBlock>
{`./rpc-proxy serve -c config.yaml`}
      </CodeBlock>

      <p>
        The <code>serve</code> command starts the proxy. The <code>-c</code> flag
        specifies the config file path (defaults to <code>config.yaml</code> in the
        current directory).
      </p>

      <h2 id="minimal-config">Minimal Config</h2>
      <p>The smallest useful config — one chain, one node:</p>
      <CodeBlock title="config.yaml">
{`server:
  port: 8080

chains:
  tezos:
    networks:
      mainnet:
        nodes:
          - name: my-node
            url: "http://localhost:8732"`}
      </CodeBlock>

      <p>
        This starts the proxy on port 8080. Tezos mainnet requests go to{' '}
        <code>http://localhost:8080/tezos/mainnet/...</code>
      </p>

      <Callout variant="tip">
        All rate limit and cache settings have sensible defaults. You only need to
        specify them if you want to override.
      </Callout>

      <h2 id="verify">Verify</h2>
      <CodeBlock>
{`# Health check
curl http://localhost:8080/tezos/mainnet/ready

# Fetch the head block header
curl http://localhost:8080/tezos/mainnet/chains/main/blocks/head/header`}
      </CodeBlock>

      <p>
        The <code>/ready</code> endpoint returns 200 if at least one node is healthy,
        503 otherwise.
      </p>

      <h2 id="next-steps">Next Steps</h2>
      <ul>
        <li>
          <a href="/configuration">Configuration</a> — full reference for all config
          fields
        </li>
        <li>
          <a href="/deployment">Deployment</a> — systemd setup and zero-downtime
          upgrades
        </li>
      </ul>
    </>
  )
}
