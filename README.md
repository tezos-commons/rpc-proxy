# rpc-proxy

High-performance RPC reverse proxy for Tezos and EVM (Etherlink) chains. Load-balances across backend nodes with caching, per-IP rate limiting, and archive-aware routing.

Routes: `/tezos/<network>/...` and `/etherlink/<network>/...`

## Build & Run

```bash
go build -o rpc-proxy .
./rpc-proxy serve -c config.yaml
```

## systemd (zero-downtime upgrades)

rpc-proxy uses [tableflip](https://github.com/cloudflare/tableflip) for graceful binary upgrades via `SIGUSR2`. The old process hands its listening socket to the new one and drains in-flight requests before exiting.

```ini
# /etc/systemd/system/rpc-proxy.service
[Unit]
Description=RPC Proxy
After=network.target

[Service]
Type=notify
ExecStart=/usr/local/bin/rpc-proxy serve -c /etc/rpc-proxy/config.yaml
ExecReload=/bin/kill -USR2 $MAINPID
Restart=on-failure
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
```

Deploy a new binary, then:

```bash
sudo systemctl reload rpc-proxy   # zero-downtime upgrade (SIGUSR2)
```

Config reload (no binary change):

```bash
sudo kill -HUP $(pidof rpc-proxy)  # or just edit the file — it's polled every 30s
```

## Configuration

```yaml
server:
  port: 8080

rate_limits:
  default: 300    # read-only chain data
  expensive: 20   # eth_call, eth_estimateGas, eth_getLogs, big_maps, raw context
  injection: 10   # eth_sendRawTransaction, /injection/operation
  script: 5       # run_code, trace_code, typecheck_*, simulate_operation
  streaming: 5    # /monitor/*, mempool monitor
  debug: 1        # debug_trace*, tez_replayBlock

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
            archive: true
      testnet:
        nodes:
          - name: node-6
            url: "http://10.0.0.6:8545"
```

Hot-reloadable fields (no restart needed): `rate_limits`, `cache_max_entries`, `server.max_streams`.
Everything else requires a restart.

## Features

**Multi-chain routing** — Serves both Tezos (REST) and Etherlink (JSON-RPC) from a single process. Requests are routed by URL prefix (`/tezos/<network>/`, `/etherlink/<network>/`).

**Load balancing with health tracking** — Round-robins across backend nodes. Each node is continuously health-checked; unhealthy nodes are removed from rotation until they recover.

**Archive-aware routing** — Requests for historical data (old blocks, past contract state) are automatically routed to nodes marked `archive: true`. Non-archive nodes only serve recent data.

**Generation-based caching** — In-memory sharded cache that automatically invalidates when a new block arrives. Each head update bumps a generation counter; stale entries are swept in the background.

**Tiered rate limiting** — Six tiers from high-throughput reads (300/s) down to debug traces (1/s). Limits are per-IP, token-bucket based, and hot-reloadable.

**Streaming support** — Long-lived SSE/event-stream connections (Tezos `/monitor/*`, EVM subscriptions) with per-IP concurrency limits.

**Fallback nodes** — Optional external fallback URLs per network, tried before returning 502 when all primary nodes are down.

**Hot config reload** — Rate limits and cache size update on `SIGHUP` or automatically when the config file changes (polled every 30s).

**Zero-downtime upgrades** — `tableflip` passes the listening socket to the new binary on `SIGUSR2`. In-flight requests drain gracefully.

**systemd notify** — Sends `READY=1` to systemd's notify socket so `Type=notify` services work correctly.
