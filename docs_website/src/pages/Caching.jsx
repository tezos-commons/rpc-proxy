import Callout from '../components/Callout'

export default function Caching() {
  return (
    <>
      <h1>Caching</h1>
      <p className="subtitle">
        Generation-based in-memory cache with automatic invalidation
      </p>

      <h2 id="how-it-works">How It Works</h2>
      <p>
        Each network has its own in-memory cache. Every cache entry is stamped with a{' '}
        <strong>generation number</strong>. When a new block arrives, the balancer
        increments the generation counter, and all entries from previous generations
        become stale.
      </p>
      <p>
        This means the cache is always consistent with the current chain head — no TTLs
        to tune, no manual invalidation.
      </p>

      <h2 id="what-gets-cached">What Gets Cached</h2>
      <ul>
        <li>Only <strong>2xx responses</strong> are cached</li>
        <li>
          <strong>Tezos</strong> — all GET requests except streaming endpoints
        </li>
        <li>
          <strong>EVM</strong> — all JSON-RPC methods except injection and streaming
        </li>
        <li>
          Injection tier (<code>eth_sendRawTransaction</code>,{' '}
          <code>/injection/operation</code>) and streaming endpoints are{' '}
          <strong>never cached</strong>
        </li>
      </ul>

      <h2 id="seen-twice">Seen-Twice Policy</h2>
      <p>
        To avoid polluting the cache with one-off requests, a key must be{' '}
        <strong>seen at least twice</strong> in the same generation before the response
        is stored. The "seen" set is capped at 32&times; the cache max size to prevent
        memory exhaustion from unique keys.
      </p>

      <h2 id="singleflight">Singleflight Deduplication</h2>
      <p>
        When multiple clients request the same data simultaneously, only the first
        request (the "leader") is forwarded upstream. All other concurrent requests for
        the same cache key wait for the leader's response. This dramatically reduces
        upstream load during traffic spikes.
      </p>
      <Callout variant="info">
        Only the leader's result is stored in cache. If the response was shared (served
        to waiters), it is not cached — the next request will become the new leader.
      </Callout>

      <h2 id="cache-keys">Cache Keys</h2>
      <ul>
        <li>
          <strong>Tezos</strong> — hashed from: network, HTTP method, upstream path,
          query string, POST body
        </li>
        <li>
          <strong>EVM</strong> — hashed from: network, JSON-RPC method, whitespace-
          compacted params (the <code>id</code> field is excluded so different callers
          share cache entries)
        </li>
      </ul>

      <h2 id="gzip">Gzip Compression</h2>
      <p>
        Responses above a size threshold are pre-compressed with gzip and stored
        alongside the uncompressed body:
      </p>
      <ul>
        <li><strong>Tezos</strong> — threshold: 256 bytes</li>
        <li><strong>EVM</strong> — threshold: 1024 bytes (higher because of id patching overhead)</li>
      </ul>
      <p>
        If the client accepts gzip, the pre-compressed version is served directly with
        no per-request compression cost.
      </p>

      <h2 id="evm-id-patching">EVM ID Patching</h2>
      <p>
        JSON-RPC clients include an <code>id</code> field that must be echoed back. Since
        cache keys exclude the id, the proxy stores the JSON-RPC <code>result</code> and{' '}
        <code>error</code> fields separately and reconstructs the response with the
        caller's id via byte concatenation — no JSON re-parsing needed.
      </p>
      <p>
        When the caller's id is <code>null</code> (common with fire-and-forget calls),
        the pre-compressed gzip response is served directly without any patching.
      </p>

      <h2 id="sweep">Cache Sweep</h2>
      <p>
        A background goroutine sweeps the cache every <strong>10 seconds</strong>:
      </p>
      <ul>
        <li>Removes entries from previous generations (stale)</li>
        <li>Reconciles the size counter to prevent drift</li>
        <li>Compacts only modified shards to release bucket memory</li>
      </ul>

      <h2 id="configuration">Configuration</h2>
      <p>
        The cache size is controlled by <code>cache_max_entries</code> (default: 10,000
        per network). This is hot-reloadable — changes take effect without restart.
      </p>
    </>
  )
}
