# Tezos Public RPC Proxy — Route Reference

Design document for a public-facing RPC proxy that sits in front of an internal Octez node.
Each route is annotated with an **exposure recommendation**:

| Label | Meaning |
|-------|---------|
| **PUBLIC** | Safe to expose — read-only blockchain data |
| **PUBLIC (rate-limit)** | Safe but computationally expensive or streaming; apply rate limits |
| **RESTRICTED** | Needed by wallets/dApps but requires rate-limiting and/or auth |
| **PRIVATE** | Must NOT be exposed — internal/admin, mutates node state, or leaks infrastructure details |

> **Scope.** This document covers the **Octez Layer-1 node** (`octez-node`) RPCs.
> Separate sections cover EVM Node, DAL Node, and Smart Rollup Node RPCs.

---

## Table of Contents

1. [Octez Node — Shell RPCs](#1-octez-node--shell-rpcs)
   - [Version & Health](#11-version--health)
   - [Config](#12-config)
   - [Monitor](#13-monitor)
   - [Protocols](#14-protocols)
   - [Injection](#15-injection)
   - [Chains](#16-chains)
   - [Blocks](#17-blocks)
   - [Mempool](#18-mempool)
   - [Workers](#19-workers)
   - [Stats & GC](#110-stats--gc)
   - [BLS](#111-bls)
   - [Network / P2P](#112-network--p2p)
2. [Octez Node — Protocol RPCs](#2-octez-node--protocol-rpcs)
   - [Constants](#21-constants)
   - [Contracts](#22-contracts)
   - [Big Maps](#23-big-maps)
   - [Delegates](#24-delegates)
   - [Votes](#25-votes)
   - [Seed & Nonces](#26-seed--nonces)
   - [Liquidity Baking](#27-liquidity-baking)
   - [Cache](#28-cache)
   - [Denunciations](#29-denunciations)
   - [Adaptive Issuance](#210-adaptive-issuance)
   - [Sapling](#211-sapling)
   - [CLST](#212-clst)
   - [Smart Rollups (on-chain)](#213-smart-rollups-on-chain)
   - [DAL (on-chain)](#214-dal-on-chain)
   - [Destination / Address Registry](#215-destination--address-registry)
   - [Protocol Info](#216-protocol-info)
   - [Helpers — Scripts](#217-helpers--scripts)
   - [Helpers — Forge & Parse](#218-helpers--forge--parse)
   - [Helpers — Baking & Attestation Rights](#219-helpers--baking--attestation-rights)
   - [Helpers — Level, Cycle & Misc](#220-helpers--level-cycle--misc)
3. [EVM Node RPCs](#3-evm-node-rpcs)
   - [REST Endpoints](#31-rest-endpoints)
   - [Public JSON-RPC Methods](#32-public-json-rpc-methods)
   - [Private JSON-RPC Methods](#33-private-json-rpc-methods)
   - [Inter-Node (Peer) Services](#34-inter-node-peer-services)
4. [DAL Node RPCs](#4-dal-node-rpcs)
5. [Smart Rollup Node RPCs](#5-smart-rollup-node-rpcs)

---

## 1. Octez Node — Shell RPCs

### 1.1 Version & Health

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/version` | Node version info | **PUBLIC** |
| GET | `/health/ready` | Whether the node is ready to answer requests | **PUBLIC** |

### 1.2 Config

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/config` | Full runtime node configuration | **PRIVATE** — leaks listen addresses, bootstrap peers, data-dir paths, RPC ACLs |
| GET | `/config/network/user_activated_upgrades` | Protocol upgrade schedule | **PUBLIC** |
| GET | `/config/network/user_activated_protocol_overrides` | Protocol override list | **PUBLIC** |
| GET | `/config/network/dal` | DAL network configuration | **PUBLIC** |
| GET | `/config/history_mode` | Node history mode (full, rolling, archive) | **PUBLIC** |
| PUT | `/config/logging` | Change logging configuration at runtime | **PRIVATE** — mutates node state |

### 1.3 Monitor

All monitor endpoints are **streaming** (Server-Sent Events / chunked transfer). They hold a connection open indefinitely.

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/monitor/bootstrapped` | Stream bootstrap status and head updates | **PUBLIC (rate-limit)** — long-lived connection |
| GET | `/monitor/validated_blocks` | Stream validated-but-not-yet-applied blocks | **PUBLIC (rate-limit)** |
| GET | `/monitor/applied_blocks` | Stream applied/stored blocks | **PUBLIC (rate-limit)** |
| GET | `/monitor/heads/<chain_id>` | Stream new chain heads | **PUBLIC (rate-limit)** |
| GET | `/monitor/received_blocks/<chain_id>` | Stream newly received blocks | **PRIVATE** — internal block reception, DoS vector |
| GET | `/monitor/protocols` | Stream retrieved/compiled protocols | **PUBLIC (rate-limit)** |
| GET | `/monitor/active_chains` | Stream chain creation/destruction | **PUBLIC (rate-limit)** |

### 1.4 Protocols

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/protocols` | List of supported protocol hashes | **PUBLIC** |
| GET | `/protocols/<protocol_hash>` | Protocol interface (modules) | **PUBLIC** |
| GET | `/protocols/<protocol_hash>/environment` | Required environment version | **PUBLIC** |
| GET | `/fetch_protocol/<protocol_hash>` | Fetch a protocol from network peers | **PRIVATE** — triggers network activity |

### 1.5 Injection

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| POST | `/injection/operation` | Inject and broadcast an operation | **RESTRICTED** — essential for wallets; rate-limit heavily |
| POST | `/injection/block` | Inject and broadcast a block | **PRIVATE** — only bakers should inject blocks |
| POST | `/injection/protocol` | Inject a protocol | **PRIVATE** — only used during protocol development |
| POST | `/private/injection/operation` | Private operation injection (fewer checks) | **PRIVATE** |
| POST | `/private/injection/operations` | Private batch operation injection | **PRIVATE** |

### 1.6 Chains

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/chains/<chain_id>/chain_id` | Chain unique identifier | **PUBLIC** |
| GET | `/chains/<chain_id>/is_bootstrapped` | Bootstrap status | **PUBLIC** |
| PATCH | `/chains/<chain_id>` | Force-set bootstrapped flag | **PRIVATE** — mutates node state |
| GET | `/chains/<chain_id>/active_peers_heads` | Heads of all active peers | **PRIVATE** — leaks peer info |
| GET | `/chains/<chain_id>/delegators_contribution/<cycle>/<pkh>` | Delegator contributions to baking power | **PUBLIC** |
| GET | `/chains/<chain_id>/levels/checkpoint` | Current checkpoint level | **PUBLIC** |
| GET | `/chains/<chain_id>/levels/savepoint` | Current savepoint level | **PUBLIC** |
| GET | `/chains/<chain_id>/levels/caboose` | Current caboose level | **PUBLIC** |
| GET | `/chains/<chain_id>/blocks` | List block hashes by fitness | **PUBLIC (rate-limit)** — can return large results |
| GET | `/chains/<chain_id>/invalid_blocks` | List invalid blocks with errors | **PRIVATE** — internal diagnostics |
| GET | `/chains/<chain_id>/invalid_blocks/<block_hash>` | Errors for a specific invalid block | **PRIVATE** |
| DELETE | `/chains/<chain_id>/invalid_blocks/<block_hash>` | Remove an invalid block record | **PRIVATE** — mutates node state |
| GET | `/chains/<chain_id>/protocols` | Protocols on this chain | **PUBLIC** |
| GET | `/chains/<chain_id>/protocols/<protocol_hash>` | Protocol info for chain | **PUBLIC** |

### 1.7 Blocks

All paths below are prefixed with `/chains/<chain_id>/blocks/<block_id>`.
`<block_id>` can be: `head`, `genesis`, a block hash, a level number, `checkpoint`, `savepoint`, `caboose`, or relative offsets like `head~2`.

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | (root) | Full block info | **PUBLIC** |
| GET | `/hash` | Block hash | **PUBLIC** |
| GET | `/header` | Parsed block header | **PUBLIC** |
| GET | `/header/raw` | Raw block header bytes | **PUBLIC** |
| GET | `/header/shell` | Shell fragment of header | **PUBLIC** |
| GET | `/header/protocol_data` | Protocol data fragment | **PUBLIC** |
| GET | `/header/protocol_data/raw` | Raw protocol data bytes | **PUBLIC** |
| GET | `/metadata` | Block metadata | **PUBLIC** |
| GET | `/metadata_hash` | Hash of block metadata | **PUBLIC** |
| GET | `/protocols` | Current and next protocol | **PUBLIC** |
| GET | `/resulting_context_hash` | Context hash after applying block | **PUBLIC** |
| GET | `/operations` | All operations in the block | **PUBLIC** |
| GET | `/operations/<list_offset>` | Operations in nth validation pass | **PUBLIC** |
| GET | `/operations/<list>/<op>` | Specific operation | **PUBLIC** |
| GET | `/operation_hashes` | All operation hashes | **PUBLIC** |
| GET | `/operation_hashes/<list>` | Operation hashes in nth pass | **PUBLIC** |
| GET | `/operation_hashes/<list>/<op>` | Specific operation hash | **PUBLIC** |
| GET | `/operations_metadata_hash` | Root hash of operations metadata | **PUBLIC** |
| GET | `/operation_metadata_hashes` | All operation metadata hashes | **PUBLIC** |
| GET | `/operation_metadata_hashes/<list>` | Metadata hashes in nth pass | **PUBLIC** |
| GET | `/operation_metadata_hashes/<list>/<op>` | Specific metadata hash | **PUBLIC** |
| GET | `/context/raw/bytes` | Raw context bytes at path | **PUBLIC (rate-limit)** — can be expensive |
| GET | `/context/raw/bytes/<path..>` | Raw context bytes at sub-path | **PUBLIC (rate-limit)** |
| GET | `/context/merkle_tree` | Merkle tree of context (v1) | **PUBLIC (rate-limit)** |
| GET | `/context/merkle_tree_v2` | Irmin merkle tree (v2) | **PUBLIC (rate-limit)** |
| POST | `/helpers/forge_block_header` | Forge a block header | **PUBLIC (rate-limit)** |
| POST | `/helpers/preapply/block` | Simulate block validation | **RESTRICTED** — expensive, used by bakers |
| POST | `/helpers/preapply/operations` | Simulate operation application | **RESTRICTED** — essential for wallets; rate-limit |
| GET | `/helpers/complete/<prefix>` | Base58Check prefix completion | **PUBLIC (rate-limit)** |
| GET | `/live_blocks` | Recently-valid ancestor blocks | **PUBLIC** |

### 1.8 Mempool

All paths prefixed with `/chains/<chain_id>/mempool`.

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/pending_operations` | List pending/prevalidated operations | **PUBLIC (rate-limit)** — can be large |
| GET | `/monitor_operations` | Stream mempool operations | **PUBLIC (rate-limit)** — long-lived |
| GET | `/filter` | Get mempool filter config | **PUBLIC** |
| POST | `/filter` | Set mempool filter config | **PRIVATE** — mutates node state |
| POST | `/ban_operation` | Ban an operation from mempool | **PRIVATE** — mutates node state |
| POST | `/unban_operation` | Unban an operation | **PRIVATE** |
| POST | `/unban_all_operations` | Clear all banned operations | **PRIVATE** |
| POST | `/request_operations` | Request operations from peers | **PRIVATE** — triggers network activity |

### 1.9 Workers

All worker endpoints expose **internal node diagnostics** and should not be public.

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/workers/prevalidators` | List prevalidator workers | **PRIVATE** |
| GET | `/workers/prevalidators/<chain_id>` | Prevalidator worker state | **PRIVATE** |
| GET | `/workers/block_validator` | Block validator worker state | **PRIVATE** |
| GET | `/workers/chain_validators` | List chain validator workers | **PRIVATE** |
| GET | `/workers/chain_validators/<chain_id>` | Chain validator state | **PRIVATE** |
| GET | `/workers/chain_validators/<chain_id>/ddb` | DDB state | **PRIVATE** |
| GET | `/workers/chain_validators/<chain_id>/peers_validators` | List peer validators | **PRIVATE** |
| GET | `/workers/chain_validators/<chain_id>/peers_validators/<peer_id>` | Peer validator state | **PRIVATE** |

### 1.10 Stats & GC

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/stats/gc` | OCaml GC stats | **PRIVATE** — internal diagnostics |
| GET | `/stats/memory` | Memory usage stats | **PRIVATE** — leaks infrastructure info |
| POST | `/gc/full` | Trigger full GC cycle | **PRIVATE** — impacts node performance |

### 1.11 BLS

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| POST | `/bls/aggregate_signatures` | Aggregate BLS signatures | **PUBLIC (rate-limit)** — CPU-intensive |
| POST | `/bls/check_proof` | Check a BLS proof | **PUBLIC (rate-limit)** |
| POST | `/bls/aggregate_public_keys` | Aggregate BLS public keys | **PUBLIC (rate-limit)** |
| POST | `/bls/aggregate_proofs` | Aggregate BLS proofs | **PUBLIC (rate-limit)** |
| POST | `/bls/threshold_signatures` | Compute threshold BLS signatures | **PUBLIC (rate-limit)** |

### 1.12 Network / P2P

**All network/P2P endpoints must be PRIVATE.** They expose peer topology, connection details, and allow banning/connecting peers.

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/network/self` | Node's own peer ID | **PRIVATE** |
| GET | `/network/stat` | Network bandwidth stats | **PRIVATE** |
| GET | `/network/log` | Stream all network events | **PRIVATE** |
| PUT | `/network/points/<point>` | Connect to a peer | **PRIVATE** |
| GET | `/network/connections` | List P2P connections | **PRIVATE** |
| GET | `/network/connections/<peer_id>` | Details of a P2P connection | **PRIVATE** |
| DELETE | `/network/connections/<peer_id>` | Force-close a P2P connection | **PRIVATE** |
| GET | `/network/points` | List known P2P endpoints | **PRIVATE** |
| GET | `/network/points/<point>` | Details of a P2P point | **PRIVATE** |
| PATCH | `/network/points/<point>` | Ban/trust/open a P2P point | **PRIVATE** |
| GET | `/network/points/<point>/log` | Network events for a point | **PRIVATE** |
| GET | `/network/points/<point>/banned` | Check if point is banned | **PRIVATE** |
| GET | `/network/peers` | List all known peers | **PRIVATE** |
| GET | `/network/peers/<peer_id>` | Details about a peer | **PRIVATE** |
| PATCH | `/network/peers/<peer_id>` | Ban/trust/open a peer | **PRIVATE** |
| GET | `/network/peers/<peer_id>/log` | Network events for a peer | **PRIVATE** |
| GET | `/network/peers/<peer_id>/banned` | Check if peer is banned | **PRIVATE** |
| GET | `/network/full_stat` | Full network statistics | **PRIVATE** |
| DELETE | `/network/greylist` | Clear all greylists | **PRIVATE** |
| GET | `/network/greylist/peers` | Greylisted peers | **PRIVATE** |
| GET | `/network/greylist/ips` | Greylisted IPs | **PRIVATE** |

---

## 2. Octez Node — Protocol RPCs

All paths below are prefixed with `/chains/<chain_id>/blocks/<block_id>`.

### 2.1 Constants

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/constants` | All protocol constants | **PUBLIC** |
| GET | `/context/constants/parametric` | Parametric constants only | **PUBLIC** |
| GET | `/context/constants/errors` | Error schema for this protocol | **PUBLIC** |

### 2.2 Contracts

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/contracts` | List all contracts | **PUBLIC (rate-limit)** — large result set |
| GET | `/context/contracts/<id>` | Full contract status | **PUBLIC** |
| GET | `/context/contracts/<id>/balance` | Spendable balance (mutez) | **PUBLIC** |
| GET | `/context/contracts/<id>/spendable` | Alias for balance | **PUBLIC** |
| GET | `/context/contracts/<id>/frozen_bonds` | Frozen bonds | **PUBLIC** |
| GET | `/context/contracts/<id>/balance_and_frozen_bonds` | Balance + frozen bonds | **PUBLIC** |
| GET | `/context/contracts/<id>/spendable_and_frozen_bonds` | Alias for above | **PUBLIC** |
| GET | `/context/contracts/<id>/staked_balance` | Staked balance | **PUBLIC** |
| GET | `/context/contracts/<id>/staking_numerator` | Staking numerator | **PUBLIC** |
| GET | `/context/contracts/<id>/unstaked_frozen_balance` | Unstaked frozen balance | **PUBLIC** |
| GET | `/context/contracts/<id>/unstaked_finalizable_balance` | Finalizable unstaked balance | **PUBLIC** |
| GET | `/context/contracts/<id>/unstake_requests` | Pending unstake requests | **PUBLIC** |
| GET | `/context/contracts/<id>/full_balance` | Full balance (all components) | **PUBLIC** |
| GET | `/context/contracts/<id>/manager_key` | Manager's public key | **PUBLIC** |
| GET | `/context/contracts/<id>/delegate` | Contract's delegate | **PUBLIC** |
| GET | `/context/contracts/<id>/counter` | Contract counter (nonce) | **PUBLIC** |
| GET | `/context/contracts/<id>/script` | Contract code and storage | **PUBLIC** |
| GET | `/context/contracts/<id>/storage` | Storage data | **PUBLIC** |
| GET | `/context/contracts/<id>/entrypoints` | List of entrypoints | **PUBLIC** |
| GET | `/context/contracts/<id>/entrypoints/<name>` | Type of specific entrypoint | **PUBLIC** |
| POST | `/context/contracts/<id>/big_map_get` | Value in big map by key (deprecated) | **PUBLIC (rate-limit)** |
| GET | `/context/contracts/<id>/estimated_own_pending_slashed_amount` | Estimated pending slash | **PUBLIC** |
| GET | `/context/contracts/<id>/single_sapling_get_diff` | Sapling state diff | **PUBLIC** |
| POST | `/context/contracts/<id>/storage/normalized` | Normalized storage | **PUBLIC (rate-limit)** |
| POST | `/context/contracts/<id>/script/normalized` | Normalized script | **PUBLIC (rate-limit)** |
| GET | `/context/contracts/<id>/storage/used_space` | Used storage space | **PUBLIC** |
| GET | `/context/contracts/<id>/storage/paid_space` | Paid storage space | **PUBLIC** |
| POST | `/context/contracts/<id>/ticket_balance` | Ticket balance query | **PUBLIC (rate-limit)** |
| GET | `/context/contracts/<id>/all_ticket_balances` | All ticket balances | **PUBLIC** |
| GET | `/context/contracts/<id>/clst_balance` | CLST token balance | **PUBLIC** |
| GET | `/context/contracts/<id>/clst_ticket_balance` | CLST ticket balance | **PUBLIC** |
| GET | `/context/contracts/<id>/clst_redeemed_frozen_balance` | CLST frozen redeem balance | **PUBLIC** |
| GET | `/context/contracts/<id>/clst_redeemed_finalizable_balance` | CLST finalizable redeem | **PUBLIC** |

### 2.3 Big Maps

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/big_maps/<id>` | All values in big map (paginated) | **PUBLIC (rate-limit)** |
| GET | `/context/big_maps/<id>/<key_hash>` | Value for a key in big map | **PUBLIC** |
| POST | `/context/big_maps/<id>/<key_hash>/normalized` | Value with normalized unparsing | **PUBLIC (rate-limit)** |

### 2.4 Delegates

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/delegates` | List all delegates (filterable) | **PUBLIC (rate-limit)** |
| GET | `/context/total_currently_staked` | Total staked tez | **PUBLIC** |
| GET | `/context/delegates/<pkh>` | Full delegate info | **PUBLIC** |
| GET | `/context/delegates/<pkh>/is_forbidden` | Whether delegate is forbidden | **PUBLIC** |
| GET | `/context/delegates/<pkh>/stakers` | Stakers and shares | **PUBLIC** |
| GET | `/context/delegates/<pkh>/own_full_balance` | Delegate's own full balance | **PUBLIC** |
| GET | `/context/delegates/<pkh>/total_staked` | Total staked (baker + external) | **PUBLIC** |
| GET | `/context/delegates/<pkh>/total_unstaked_per_cycle` | Unstaked amounts per cycle | **PUBLIC** |
| GET | `/context/delegates/<pkh>/total_delegated` | Total delegated tokens | **PUBLIC** |
| GET | `/context/delegates/<pkh>/delegators` | All delegating contracts | **PUBLIC** |
| GET | `/context/delegates/<pkh>/own_staked` | Baker's own staked amount | **PUBLIC** |
| GET | `/context/delegates/<pkh>/own_delegated` | Baker's own delegated amount | **PUBLIC** |
| GET | `/context/delegates/<pkh>/external_staked` | External stakers' total | **PUBLIC** |
| GET | `/context/delegates/<pkh>/external_delegated` | External delegators' total | **PUBLIC** |
| GET | `/context/delegates/<pkh>/staking_denominator` | Staking denominator | **PUBLIC** |
| GET | `/context/delegates/<pkh>/deactivated` | Whether deactivated | **PUBLIC** |
| GET | `/context/delegates/<pkh>/grace_period` | Grace period cycle | **PUBLIC** |
| GET | `/context/delegates/<pkh>/current_voting_power` | Voting power from current stake | **PUBLIC** |
| GET | `/context/delegates/<pkh>/voting_power` | Voting power in listings | **PUBLIC** |
| GET | `/context/delegates/<pkh>/baking_power` | Current baking power | **PUBLIC** |
| GET | `/context/delegates/<pkh>/voting_info` | Voting info | **PUBLIC** |
| GET | `/context/delegates/<pkh>/consensus_key` | Active + pending consensus keys | **PUBLIC** |
| GET | `/context/delegates/<pkh>/companion_key` | Active + pending companion keys | **PUBLIC** |
| GET | `/context/delegates/<pkh>/participation` | Participation info | **PUBLIC** |
| GET | `/context/delegates/<pkh>/dal_participation` | DAL attestation participation | **PUBLIC** |
| GET | `/context/delegates/<pkh>/active_staking_parameters` | Active staking params | **PUBLIC** |
| GET | `/context/delegates/<pkh>/pending_staking_parameters` | Pending staking params | **PUBLIC** |
| GET | `/context/delegates/<pkh>/clst_registered` | CLST registration status | **PUBLIC** |
| GET | `/context/delegates/<pkh>/active_clst_parameters` | Active CLST params | **PUBLIC** |
| GET | `/context/delegates/<pkh>/pending_clst_parameters` | Pending CLST params | **PUBLIC** |
| GET | `/context/delegates/<pkh>/denunciations` | Pending denunciations | **PUBLIC** |
| GET | `/context/delegates/<pkh>/estimated_shared_pending_slashed_amount` | Estimated shared slash | **PUBLIC** |
| GET | `/context/delegates/<pkh>/min_delegated_in_current_cycle` | Min delegated in cycle | **PUBLIC** |
| GET | `/context/delegates/<pkh>/full_balance` | DEPRECATED — use own_full_balance | **PUBLIC** |
| GET | `/context/delegates/<pkh>/current_frozen_deposits` | DEPRECATED — use total_staked | **PUBLIC** |
| GET | `/context/delegates/<pkh>/staking_balance` | DEPRECATED | **PUBLIC** |
| GET | `/context/delegates/<pkh>/total_delegated_stake` | DEPRECATED | **PUBLIC** |
| GET | `/context/delegates/<pkh>/delegated_balance` | DEPRECATED | **PUBLIC** |
| GET | `/context/delegates/<pkh>/frozen_deposits` | DEPRECATED | **PUBLIC** |
| GET | `/context/delegates/<pkh>/frozen_deposits_limit` | DEPRECATED | **PUBLIC** |
| GET | `/context/delegates/<pkh>/current_baking_power` | DEPRECATED | **PUBLIC** |
| GET | `/context/delegates/<pkh>/delegated_contracts` | DEPRECATED | **PUBLIC** |
| GET | `/context/delegates/<pkh>/unstaked_frozen_deposits` | DEPRECATED | **PUBLIC** |

### 2.5 Votes

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/votes/ballots` | Sum of ballots cast | **PUBLIC** |
| GET | `/votes/ballot_list` | All ballots cast | **PUBLIC** |
| GET | `/votes/current_period` | Current voting period info | **PUBLIC** |
| GET | `/votes/successor_period` | Next voting period info | **PUBLIC** |
| GET | `/votes/current_quorum` | Expected quorum | **PUBLIC** |
| GET | `/votes/listings` | Delegates with voting power | **PUBLIC** |
| GET | `/votes/proposals` | Proposals with supporter counts | **PUBLIC** |
| GET | `/votes/current_proposal` | Current proposal under evaluation | **PUBLIC** |
| GET | `/votes/total_voting_power` | Total voting power | **PUBLIC** |
| GET | `/votes/proposal_count/<pkh>` | Votes cast by a delegate | **PUBLIC** |

### 2.6 Seed & Nonces

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/seed_computation` | Seed computation status | **PUBLIC** |
| POST | `/context/seed` | Seed of the current cycle | **PRIVATE** — reveals randomness seed |
| GET | `/context/nonces/<raw_level>` | Nonce info for a previous block | **PUBLIC** |

### 2.7 Liquidity Baking

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/liquidity_baking/cpmm_address` | CPMM contract address | **PUBLIC** |

### 2.8 Cache

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/cache/contracts/all` | List of cached contracts | **PRIVATE** — internal cache state |
| GET | `/context/cache/contracts/size` | Contract cache size | **PRIVATE** |
| GET | `/context/cache/contracts/size_limit` | Contract cache size limit | **PUBLIC** |
| POST | `/context/cache/contracts/rank` | Rank of a contract in cache | **PRIVATE** — internal cache state |

### 2.9 Denunciations

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/denunciations` | Pending denunciations in current cycle | **PUBLIC** |

### 2.10 Adaptive Issuance

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/total_supply` | Total tez supply | **PUBLIC** |
| GET | `/context/total_frozen_stake` | Total frozen stake | **PUBLIC** |
| GET | `/context/adaptive_issuance_launch_cycle` | AI launch cycle | **PUBLIC** |
| GET | `/context/issuance/current_yearly_rate` | Current max yearly issuance rate (%) | **PUBLIC** |
| GET | `/context/issuance/current_yearly_rate_exact` | Exact yearly rate (quotient) | **PUBLIC** |
| GET | `/context/issuance/current_yearly_rate_details` | Yearly rate breakdown | **PUBLIC** |
| GET | `/context/issuance/issuance_per_minute` | Current issuance per minute | **PUBLIC** |
| GET | `/context/issuance/expected_issuance` | Expected issuance for coming cycles | **PUBLIC** |

### 2.11 Sapling

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/sapling/<id>/get_diff` | Sapling state root + diff | **PUBLIC (rate-limit)** — can be large |

### 2.12 CLST

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/clst/contract_hash` | CLST contract hash | **PUBLIC** |
| GET | `/context/clst/total_supply` | CLST total supply | **PUBLIC** |
| GET | `/context/clst/total_amount_of_tez` | Total tez in CLST ledger | **PUBLIC** |
| GET | `/context/clst/exchange_rate` | CLST/tez exchange rate | **PUBLIC** |

### 2.13 Smart Rollups (on-chain)

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/smart_rollups/all` | List all originated smart rollups | **PUBLIC** |
| GET | `/context/smart_rollups/all/inbox` | Smart rollups inbox | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/kind` | Rollup kind | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/genesis_info` | Genesis level + commitment | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/last_cemented_commitment_hash_with_level` | Last cemented commitment | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/staker/<pkh>/staked_on_commitment` | Newest staked commitment | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/commitment/<hash>` | Commitment by hash | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/dal_slot_subscriptions/<level>` | DAL slot subscriptions | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/staker/<pkh>/games` | Ongoing refutation games | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/inbox_level/<level>/commitments` | Commitments at inbox level | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/commitment/<hash>/stakers_indexes` | Staker indexes on commitment | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/staker/<pkh>/index` | Staker index | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/stakers` | All active stakers | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/staker/<pkh>/conflicts` | Conflicting stakers | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/staker1/<s1>/staker2/<s2>/timeout` | Refutation game timeout | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/staker1/<s1>/staker2/<s2>/timeout_reached` | Timeout game result | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/commitment/<hash>/can_be_cemented` | Can commitment be cemented | **PUBLIC** |
| POST | `/context/smart_rollups/smart_rollup/<addr>/ticket_balance` | Rollup ticket balance | **PUBLIC (rate-limit)** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/whitelist` | Rollup whitelist | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/last_whitelist_update` | Last whitelist update | **PUBLIC** |
| GET | `/context/smart_rollups/smart_rollup/<addr>/consumed_outputs/<level>` | Known consumed outputs | **PUBLIC** |

### 2.14 DAL (on-chain)

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/dal/commitments_history` | Last DAL skip list cell | **PUBLIC** |
| GET | `/context/dal/shards` | Shard assignment for level/delegates | **PUBLIC** |
| GET | `/context/dal/past_parameters/<level>` | DAL parameters at level | **PUBLIC** |
| GET | `/context/dal/published_slot_headers` | Published slot headers | **PUBLIC** |
| GET | `/context/dal/skip_list_cells_of_level` | DAL skip list cells | **PUBLIC** |

### 2.15 Destination / Address Registry

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/destination/<dest>/index` | Index assigned by INDEX_ADDRESS | **PUBLIC** |

### 2.16 Protocol Info

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/context/protocol/first_level` | Level at which protocol was activated | **PUBLIC** |

### 2.17 Helpers — Scripts

These endpoints execute Michelson code on the node. They are **CPU-intensive** and potential DoS vectors.

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| POST | `/helpers/scripts/run_code` | Run Michelson script in current context | **RESTRICTED** — CPU-intensive, rate-limit heavily |
| POST | `/helpers/scripts/trace_code` | Run script with execution trace | **RESTRICTED** |
| POST | `/helpers/scripts/run_view` | Simulate TZIP-4 view call | **RESTRICTED** |
| POST | `/helpers/scripts/run_script_view` | Simulate Michelson view call | **RESTRICTED** |
| POST | `/helpers/scripts/run_instruction` | Run single Michelson instruction | **RESTRICTED** |
| POST | `/helpers/scripts/typecheck_code` | Typecheck code | **RESTRICTED** |
| POST | `/helpers/scripts/script_size` | Compute script size | **PUBLIC (rate-limit)** |
| POST | `/helpers/scripts/typecheck_data` | Typecheck data expression | **RESTRICTED** |
| POST | `/helpers/scripts/pack_data` | Serialize data with PACK | **PUBLIC (rate-limit)** |
| POST | `/helpers/scripts/normalize_data` | Normalize data expression | **PUBLIC (rate-limit)** |
| POST | `/helpers/scripts/normalize_stack` | Normalize Michelson stack | **PUBLIC (rate-limit)** |
| POST | `/helpers/scripts/normalize_script` | Normalize Michelson script | **PUBLIC (rate-limit)** |
| POST | `/helpers/scripts/normalize_type` | Normalize Michelson type | **PUBLIC (rate-limit)** |
| POST | `/helpers/scripts/run_operation` | Run operation (no sig checks) | **RESTRICTED** — simulates operations |
| POST | `/helpers/scripts/simulate_operation` | Simulate operation at future time | **RESTRICTED** |
| POST | `/helpers/scripts/entrypoint` | Return entrypoint type | **PUBLIC (rate-limit)** |
| POST | `/helpers/scripts/entrypoints` | List all entrypoints | **PUBLIC (rate-limit)** |

### 2.18 Helpers — Forge & Parse

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| POST | `/helpers/forge/operations` | Forge unsigned operation bytes | **PUBLIC (rate-limit)** |
| POST | `/helpers/forge/signed_operations` | Forge signed operation bytes | **PUBLIC (rate-limit)** |
| POST | `/helpers/forge/protocol_data` | Forge protocol data of block header | **PUBLIC (rate-limit)** |
| POST | `/helpers/forge/bls_consensus_operations` | Forge BLS consensus operation | **PUBLIC (rate-limit)** |
| POST | `/helpers/parse/operations` | Parse operations from bytes | **PUBLIC (rate-limit)** |
| POST | `/helpers/parse/block` | Parse block from bytes | **PUBLIC (rate-limit)** |

### 2.19 Helpers — Baking & Attestation Rights

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/helpers/baking_rights` | Baking rights (filterable) | **PUBLIC** |
| GET | `/helpers/attestation_rights` | Attestation rights (filterable) | **PUBLIC** |
| GET | `/helpers/validators` | Attestation slots per delegate | **PUBLIC** |

### 2.20 Helpers — Level, Cycle & Misc

| Method | Path suffix | Description | Exposure |
|--------|-------------|-------------|----------|
| GET | `/helpers/current_level` | Current level (with optional offset) | **PUBLIC** |
| GET | `/helpers/levels_in_current_cycle` | First/last levels of current cycle | **PUBLIC** |
| GET | `/helpers/round` | Current round | **PUBLIC** |
| GET | `/helpers/consecutive_round_zero` | Consecutive round-0 blocks | **PUBLIC** |
| GET | `/helpers/total_baking_power` | Total baking power for cycle | **PUBLIC** |
| GET | `/helpers/baking_power_distribution_for_current_cycle` | Full baking power distribution | **PUBLIC** |
| GET | `/helpers/swrr_selected_bakers` | SWRR-selected bakers for round 0 | **PUBLIC** |
| GET | `/helpers/swrr_credits` | SWRR credits for all delegates | **PUBLIC** |
| GET | `/helpers/tz4_baker_number_ratio` | Ratio of tz4-key bakers | **PUBLIC** |
| GET | `/helpers/all_bakers_attest_activation_level` | All Bakers Attest activation level | **PUBLIC** |
| GET | `/helpers/decode_dal_attestation/<bitset>` | Decode DAL attestation bitset | **PUBLIC** |
| POST | `/helpers/encode_dal_attestation` | Encode DAL attestation to bitset | **PUBLIC** |

---

## 3. EVM Node RPCs

The EVM node (`octez-evm-node`) serves Ethereum-compatible JSON-RPC plus Tezos-specific REST endpoints.

### 3.1 REST Endpoints

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/version` | EVM node version | **PUBLIC** |
| GET | `/health_check` | Health status (query: `drift_threshold`) | **PUBLIC** |
| GET | `/configuration` | Node config (sensitive fields hidden) | **PRIVATE** — leaks config details |
| GET | `/mode` | Operating mode (sequencer/observer/rpc) | **PRIVATE** — leaks topology |
| POST | `/` | Public JSON-RPC batch dispatcher | **PUBLIC (rate-limit)** |
| GET | `/ws` | Public WebSocket JSON-RPC | **PUBLIC (rate-limit)** |
| POST | `/private` | Private JSON-RPC dispatcher | **PRIVATE** |
| GET | `/private/ws` | Private WebSocket endpoint | **PRIVATE** |
| GET | `/metrics` | Prometheus metrics | **PRIVATE** — internal monitoring |

### 3.2 Public JSON-RPC Methods

Served via `POST /` and `GET /ws`.

| Method | Description | Exposure |
|--------|-------------|----------|
| `net_version` | Network ID | **PUBLIC** |
| `eth_chainId` | Chain ID (EIP-155) | **PUBLIC** |
| `eth_accounts` | Returns `[]` | **PUBLIC** |
| `eth_blockNumber` | Latest block number | **PUBLIC** |
| `eth_getBlockByNumber` | Block by number | **PUBLIC** |
| `eth_getBlockByHash` | Block by hash | **PUBLIC** |
| `eth_getBlockReceipts` | All receipts for a block | **PUBLIC** |
| `eth_getBalance` | Account balance | **PUBLIC** |
| `eth_getStorageAt` | Storage slot value | **PUBLIC** |
| `eth_getCode` | Contract bytecode | **PUBLIC** |
| `eth_getTransactionCount` | Account nonce | **PUBLIC** |
| `eth_getTransactionReceipt` | Transaction receipt | **PUBLIC** |
| `eth_getTransactionByHash` | Transaction by hash | **PUBLIC** |
| `eth_getTransactionByBlockHashAndIndex` | Tx by block hash + index | **PUBLIC** |
| `eth_getTransactionByBlockNumberAndIndex` | Tx by block number + index | **PUBLIC** |
| `eth_getBlockTransactionCountByHash` | Tx count by block hash | **PUBLIC** |
| `eth_getBlockTransactionCountByNumber` | Tx count by block number | **PUBLIC** |
| `eth_getUncleCountByBlockHash` | Always 0 | **PUBLIC** |
| `eth_getUncleCountByBlockNumber` | Always 0 | **PUBLIC** |
| `eth_getUncleByBlockHashAndIndex` | Always null | **PUBLIC** |
| `eth_getUncleByBlockNumberAndIndex` | Always null | **PUBLIC** |
| `eth_gasPrice` | Current gas price | **PUBLIC** |
| `eth_maxPriorityFeePerGas` | Max priority fee (EIP-1559) | **PUBLIC** |
| `eth_feeHistory` | Historical fee data | **PUBLIC** |
| `eth_coinbase` | Sequencer address | **PUBLIC** |
| `eth_sendRawTransaction` | Broadcast signed tx (async) | **RESTRICTED** — rate-limit |
| `eth_sendRawTransactionSync` | Broadcast signed tx (sync wait) | **RESTRICTED** — rate-limit, blocks connection |
| `eth_call` | Simulate call (no state change) | **RESTRICTED** — CPU-intensive |
| `eth_estimateGas` | Estimate gas | **RESTRICTED** — CPU-intensive |
| `eth_getLogs` | Query event logs | **PUBLIC (rate-limit)** — can scan many blocks |
| `eth_subscribe` | Subscribe to events (WS only) | **PUBLIC (rate-limit)** |
| `eth_unsubscribe` | Unsubscribe (WS only) | **PUBLIC** |
| `txpool_content` | Mempool contents | **PUBLIC (rate-limit)** |
| `web3_clientVersion` | Client version | **PUBLIC** |
| `web3_sha3` | Keccak-256 hash | **PUBLIC** |
| `debug_traceTransaction` | Debug trace a transaction | **RESTRICTED** — very CPU-intensive |
| `debug_traceCall` | Debug trace a call | **RESTRICTED** — very CPU-intensive |
| `debug_traceBlockByNumber` | Debug trace entire block | **RESTRICTED** — extremely CPU-intensive |
| `http_traceCall` | HTTP-mode trace call | **RESTRICTED** |
| `stateValue` | Read durable storage value | **PUBLIC** |
| `stateSubkeys` | Read durable storage subkeys | **PUBLIC** |
| `tez_chainFamily` | Chain family for chain ID | **PUBLIC** |
| `tez_blockNumber` | Generic block number | **PUBLIC** |
| `tez_kernelVersion` | WASM kernel version | **PUBLIC** |
| `tez_kernelRootHash` | Kernel root hash | **PUBLIC** |
| `tez_sequencer` | Sequencer pubkey at block | **PUBLIC** |
| `tez_getTransactionGasInfo` | Gas breakdown for tx | **PUBLIC** |
| `tez_getFinalizedBlocksOfL1Level` | L2 blocks finalized by L1 level | **PUBLIC** |
| `tez_sendRawTezlinkOperation` | Broadcast Tezlink operation | **RESTRICTED** — rate-limit |
| `tez_replayBlock` | Replay a block (debug) | **RESTRICTED** — CPU-intensive |
| `tez_getTezosEthereumAddress` | Tezos → Ethereum address map | **PUBLIC** |
| `tez_getEthereumTezosAddress` | Ethereum → Tezos address map | **PUBLIC** |

### 3.3 Private JSON-RPC Methods

Served via `POST /private`. **All PRIVATE — must never be exposed.**

| Method | Description |
|--------|-------------|
| `produceBlock` | Force block production (sequencer) |
| `proposeNextBlockTimestamp` | Set next block timestamp |
| `produceProposal` | Produce block proposal |
| `executeSingleTransaction` | Execute tx via IC path |
| `injectTransaction` | Inject pre-validated tx |
| `waitTransactionConfirmation` | Wait for tx confirmation |
| `injectTezlinkOperation` | Inject Tezlink operation |
| `lockBlockProduction` | Pause block production |
| `unlockBlockProduction` | Resume block production |

### 3.4 Inter-Node (Peer) Services

Served under `/evm/...`. Used for node-to-node communication. **All PRIVATE — internal infrastructure.**

| Method | Path | Description |
|--------|------|-------------|
| GET | `/evm/smart_rollup_address` | Smart rollup address |
| GET | `/evm/time_between_blocks` | Max time between blocks |
| GET | `/evm/blueprint/<level>` | Blueprint at level (legacy) |
| GET | `/evm/v2/blueprint/<level>` | Blueprint with full events |
| GET | `/evm/blueprints/range` | Range of blueprints |
| GET | `/evm/v2/blueprints/range` | Range of blueprints (v2) |
| GET | `/evm/blueprints` | Stream new blueprints |
| GET | `/evm/messages` | Stream broadcast messages |

---

## 4. DAL Node RPCs

The DAL node (`octez-dal-node`) serves data-availability-layer specific endpoints.

### Public-safe endpoints

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/health` | Health check | **PUBLIC** |
| GET | `/version` | DAL node version | **PUBLIC** |
| GET | `/synchronized` | L1 sync status | **PUBLIC** |
| GET | `/monitor/synchronized` | Stream L1 sync status | **PUBLIC (rate-limit)** |
| GET | `/last_processed_level` | Last finalized L1 level | **PUBLIC** |
| GET | `/protocol_parameters` | Protocol parameters | **PUBLIC** |
| GET | `/profiles` | Current tracked profiles | **PUBLIC** |
| GET | `/levels/<level>/slots/<idx>/commitment` | Slot commitment | **PUBLIC** |
| GET | `/levels/<level>/slots/<idx>/status` | Slot attestation status | **PUBLIC** |
| GET | `/levels/<level>/slots/<idx>/content` | Full slot content | **PUBLIC (rate-limit)** — large data |
| GET | `/levels/<level>/slots/<idx>/pages` | Slot pages | **PUBLIC (rate-limit)** |
| GET | `/levels/<level>/slots/<idx>/pages/<page>/proof` | Page proof | **PUBLIC (rate-limit)** |
| GET | `/levels/<level>/slots/<idx>/shards/<shard>/content` | Shard content | **PUBLIC (rate-limit)** |
| GET | `/profiles/<pkh>/attested_levels/<level>/assigned_shard_indices` | Shard indices | **PUBLIC** |
| GET | `/profiles/<pkh>/attested_levels/<level>/attestable_slots` | Attestable slots | **PUBLIC** |
| GET | `/profiles/<pkh>/monitor/attestable_slots` | Stream attestable slots | **PUBLIC (rate-limit)** |
| GET | `/published_levels/<level>/known_traps` | Known traps at level | **PUBLIC** |

### Private endpoints

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| POST | `/slots` | Post a slot (compute commitment/proof) | **PRIVATE** — mutates state |
| PATCH | `/profiles` | Update tracked profiles | **PRIVATE** — mutates config |
| POST | `/p2p/connect` | Connect to P2P peer | **PRIVATE** |
| DELETE | `/p2p/points/disconnect/<point>` | Disconnect from point | **PRIVATE** |
| GET | `/p2p/points` | List P2P points | **PRIVATE** — leaks topology |
| GET | `/p2p/points/info` | Points with info | **PRIVATE** |
| GET | `/p2p/points/by-id/<point>` | Point info | **PRIVATE** |
| DELETE | `/p2p/peers/disconnect/<peer_id>` | Disconnect peer | **PRIVATE** |
| GET | `/p2p/peers` | List peers | **PRIVATE** |
| GET | `/p2p/peers/info` | Peers with info | **PRIVATE** |
| GET | `/p2p/peers/by-id/<peer_id>` | Peer info | **PRIVATE** |
| PATCH | `/p2p/peers/by-id/<peer_id>` | Ban/trust/open peer | **PRIVATE** |
| GET | `/p2p/gossipsub/*` | All gossipsub introspection | **PRIVATE** |

---

## 5. Smart Rollup Node RPCs

The smart rollup node (`octez-smart-rollup-node`) serves rollup-specific endpoints.

### Public-safe endpoints

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/ping` | Node alive check | **PUBLIC** |
| GET | `/health` | Health status | **PUBLIC** |
| GET | `/version` | Version info | **PUBLIC** |
| GET | `/openapi` | OpenAPI spec | **PUBLIC** |
| GET | `/global/smart_rollup_address` | Rollup address | **PUBLIC** |
| GET | `/global/tezos_head` | Latest L1 block hash | **PUBLIC** |
| GET | `/global/tezos_level` | Latest L1 level | **PUBLIC** |
| GET | `/global/last_stored_commitment` | Last computed commitment | **PUBLIC** |
| GET | `/global/last_cemented_commitment` | Last cemented commitment | **PUBLIC** |
| GET | `/global/monitor_blocks` | Stream L2 blocks | **PUBLIC (rate-limit)** |
| GET | `/global/monitor_finalized_blocks` | Stream finalized blocks | **PUBLIC (rate-limit)** |
| GET | `/global/block/<id>` | Full L2 block | **PUBLIC** |
| GET | `/global/block/<id>/hash` | L1 block hash | **PUBLIC** |
| GET | `/global/block/<id>/level` | L1 level | **PUBLIC** |
| GET | `/global/block/<id>/inbox` | Rollup inbox | **PUBLIC** |
| GET | `/global/block/<id>/ticks` | PVM ticks | **PUBLIC** |
| GET | `/global/block/<id>/total_ticks` | Cumulative PVM ticks | **PUBLIC** |
| GET | `/global/block/<id>/num_messages` | Number of inbox messages | **PUBLIC** |
| GET | `/global/block/<id>/state_hash` | PVM state hash | **PUBLIC** |
| GET | `/global/block/<id>/state_current_level` | PVM current level | **PUBLIC** |
| GET | `/global/block/<id>/state` | PVM state value by key | **PUBLIC (rate-limit)** |
| GET | `/global/block/<id>/committed_status` | Commitment status | **PUBLIC** |

### Private endpoints

| Method | Path | Description | Exposure |
|--------|------|-------------|----------|
| GET | `/config` | Node configuration | **PRIVATE** |
| GET | `/stats/ocaml_gc` | OCaml GC stats | **PRIVATE** |
| GET | `/stats/memory` | Memory stats | **PRIVATE** |
| GET | `/local/last_published_commitment` | Last published commitment | **PRIVATE** — operator-specific |
| GET | `/local/commitments/<hash>` | Commitment details | **PRIVATE** |
| GET | `/local/outbox/pending` | Pending outbox messages | **PRIVATE** |
| GET | `/local/outbox/pending/executable` | Executable outbox messages | **PRIVATE** |
| GET | `/local/outbox/pending/unexecutable` | Non-executable outbox messages | **PRIVATE** |
| GET | `/local/gc_info` | GC info | **PRIVATE** |
| POST | `/local/batcher/injection` | Inject L2 messages | **PRIVATE** |
| GET | `/local/batcher/queue` | Batcher queue | **PRIVATE** |
| GET | `/local/batcher/queue/<id>` | Batcher message status | **PRIVATE** |
| POST | `/local/dal/batcher/injection` | Inject DAL messages | **PRIVATE** |
| GET | `/local/injector/operation/<id>/status` | Injector op status | **PRIVATE** |
| GET | `/local/dal/injected/operations/statuses` | DAL injection statuses | **PRIVATE** |
| POST | `/local/dal/injection/<id>/forget` | Forget DAL injection | **PRIVATE** |
| POST | `/local/dal/slot/indices` | Update DAL slot indices | **PRIVATE** |
| GET | `/local/synchronized` | Sync progress stream | **PRIVATE** |
| GET | `/admin/injector/queues/total` | Injector queue total | **PRIVATE** |
| GET | `/admin/injector/queues` | Injector queues | **PRIVATE** |
| DELETE | `/admin/injector/queues` | Clear injector queues | **PRIVATE** |
| GET | `/admin/cancel_gc` | Cancel GC | **PRIVATE** |
| DELETE | `/admin/batcher/queue` | Clear batcher queue | **PRIVATE** |

---

## Quick-Reference: Proxy Deny List

For a minimal safe proxy, **block all of the following path prefixes** and allow everything else with rate-limiting:

```
# Octez Node — must block
/network/**                          # All P2P/peer endpoints
/workers/**                          # Internal worker diagnostics
/stats/**                            # Memory/GC stats
/gc/**                               # GC trigger
/config (GET root only)              # Full config dump
/config/logging (PUT)                # Logging mutation
/private/**                          # Private injection endpoints
/injection/block                     # Block injection
/injection/protocol                  # Protocol injection
/fetch_protocol/**                   # Network fetch trigger
/chains/*/invalid_blocks (DELETE)    # Mutation
/chains/*/active_peers_heads         # Peer info leak
/chains/*  (PATCH)                   # Force bootstrap flag
/chains/*/mempool/ban_operation      # Mempool mutation
/chains/*/mempool/unban_operation    # Mempool mutation
/chains/*/mempool/unban_all_operations
/chains/*/mempool/filter (POST)      # Mempool config mutation
/chains/*/mempool/request_operations # Network trigger
/monitor/received_blocks/**          # Internal block reception
/context/seed (POST)                 # Reveals randomness seed
/context/cache/contracts/all         # Internal cache state
/context/cache/contracts/size        # Internal cache state
/context/cache/contracts/rank (POST) # Internal cache state

# EVM Node — must block
/private/**                          # All private JSON-RPC
/evm/**                              # Inter-node peer services
/configuration                       # Config dump
/mode                                # Topology leak
/metrics                             # Prometheus metrics

# DAL Node — must block
/p2p/**                              # All P2P endpoints
/slots (POST)                        # Slot posting
/profiles (PATCH)                    # Profile mutation

# Smart Rollup Node — must block
/config                              # Config dump
/stats/**                            # Internal stats
/local/**                            # All local/operator endpoints
/admin/**                            # All admin endpoints
```

### Recommended Rate Limits

| Category | Suggested limit | Endpoints |
|----------|----------------|-----------|
| Read-only chain data | 100 req/s per IP | Block, header, operations, delegates, constants |
| Streaming | 5 concurrent per IP | `/monitor/*`, mempool monitor, WebSocket |
| Operation injection | 10 req/s per IP | `/injection/operation`, `eth_sendRawTransaction` |
| Script execution | 5 req/s per IP | `run_code`, `run_view`, `run_operation`, `simulate_operation` |
| Debug tracing | 1 req/s per IP | `debug_trace*`, `http_traceCall` |
| Expensive queries | 20 req/s per IP | `eth_getLogs`, `eth_call`, `eth_estimateGas`, big_maps, raw context |
