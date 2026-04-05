import CodeBlock from '../components/CodeBlock'
import Callout from '../components/Callout'

export default function Routing() {
  return (
    <>
      <h1>Routing</h1>
      <p className="subtitle">How requests are mapped to upstream nodes</p>

      <h2 id="url-structure">URL Structure</h2>
      <p>
        All requests are routed by URL prefix. The proxy strips the prefix and forwards
        the remaining path to the upstream node.
      </p>
      <CodeBlock>
{`# Tezos
GET /tezos/<network>/chains/main/blocks/head/header
    → forwards to: <node>/chains/main/blocks/head/header

# Etherlink (EVM)
POST /etherlink/<network>/
    → forwards JSON-RPC body to: <node>/`}
      </CodeBlock>

      <p>
        The <code>&lt;network&gt;</code> segment matches a key in your config's{' '}
        <code>chains.tezos.networks</code> or <code>chains.etherlink.networks</code> map
        (e.g., <code>mainnet</code>, <code>ghostnet</code>, <code>testnet</code>).
        Unknown networks return <strong>404</strong>.
      </p>

      <h2 id="request-flow">Request Flow</h2>
      <h3 id="tezos-flow">Tezos</h3>
      <ol>
        <li>Parse URL: extract network and upstream path</li>
        <li>Check if the route is blocked → 403 Forbidden</li>
        <li>Check if it's a streaming endpoint → stream handler</li>
        <li>Rate limit check (per tier) → 429 Too Many Requests</li>
        <li>Archive check: does this path need an archive node?</li>
        <li>Cache lookup → serve cached if hit</li>
        <li>Singleflight dedup + forward to upstream (up to 3 retries, 150ms apart)</li>
        <li>All nodes fail → try fallback URLs → 502</li>
      </ol>

      <h3 id="evm-flow">EVM (Etherlink)</h3>
      <ol>
        <li>Parse URL: extract network</li>
        <li>Check REST path (blocked paths like <code>/private</code>) → 403</li>
        <li>Parse JSON-RPC body (method, params, id)</li>
        <li>Check if method is blocked → JSON-RPC error</li>
        <li>Rate limit check (per tier) → JSON-RPC error</li>
        <li>Archive check based on method and block parameter</li>
        <li>Cache lookup → serve cached (with id patching) if hit</li>
        <li>Singleflight dedup + forward (up to 3 retries)</li>
        <li>Fallback attempt → JSON-RPC error</li>
      </ol>

      <Callout variant="info">
        EVM supports <strong>batch requests</strong> (JSON arrays). Each item in the
        batch is processed independently through the full pipeline — rate limiting,
        caching, and forwarding happen per-item. Max batch size is 100.
      </Callout>

      <h2 id="cors">CORS</h2>
      <p>All responses include permissive CORS headers:</p>
      <CodeBlock>
{`Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: *
Access-Control-Allow-Headers: *
Access-Control-Max-Age: 3600
X-Proxy: tc-rpc-proxy`}
      </CodeBlock>
      <p>
        <code>OPTIONS</code> requests are handled directly with a <strong>204 No
        Content</strong> response.
      </p>

      <h2 id="health">Health Endpoint</h2>
      <p>
        Each network exposes a <code>/ready</code> endpoint:
      </p>
      <CodeBlock>
{`GET /tezos/mainnet/ready     → 200 if ≥1 healthy node, 503 otherwise
GET /etherlink/mainnet/ready → 200 if ≥1 healthy node, 503 otherwise`}
      </CodeBlock>
      <p>
        This endpoint does not include CORS headers and is intended for load balancer
        health checks.
      </p>

      <h2 id="client-ip">Client IP Detection</h2>
      <p>
        The client IP is extracted in this order of priority (for rate limiting and
        stream tracking):
      </p>
      <ol>
        <li><code>X-Real-IP</code> header (preferred — set by your reverse proxy)</li>
        <li><code>X-Forwarded-For</code> header (first entry)</li>
        <li>TCP remote address (fallback)</li>
      </ol>

      <h2 id="base-path">Base Path Redirect</h2>
      <p>
        A GET request to the bare network path (e.g., <code>/tezos/mainnet</code> or{' '}
        <code>/etherlink/mainnet</code>) returns a <strong>301 redirect</strong> to the
        domain root.
      </p>
    </>
  )
}
