# RosPanel: Architectural Roadmap & Large Changes (TODO)

This document tracks large-scale architectural and performance enhancements identified during the **Go 1.27.x** audit that involve dependency additions, protocol changes, or breaking contract considerations.

---

## 1. Direct In-Process RPC Client for Xray StatsService

- **Priority**: High (Architecture)
- **Status**: Completed
- **Target Components**: [`internal/xray/supervisor.go`](internal/xray/supervisor.go), [`internal/core/manager_stats.go`](internal/core/manager_stats.go), [`internal/xray/statsrpc/`](internal/xray/statsrpc/)

### Background & Current State
- `Supervisor.QueryStats(apiAddr)` and `Supervisor.PingAPI(apiAddr)` previously invoked `exec.CommandContext` to run the `xray api statsquery` CLI binary.
- A 1.5s coalescing cache (`statsMu`, `lastStatsTime`, `lastStatsVal`) coalesced overlapping queries between `vpnSpeedLoop` (3s), status feeds (2s), and admin requests.
- While concurrent forks were eliminated, periodic query execution still spawned a new process when the cache expired.

### Implemented Architecture
- Implemented an in-process gRPC client connecting directly to Xray's API port (`127.0.0.1:<api_port>`) using `StatsServiceClient.QueryStats`.
- Query `StatsService/QueryStats` over a persistent TCP loopback connection using unary RPC with 3s timeout and automatic reconnection.
- Completely eliminated `fork()` and `execve()` overhead in both `QueryStats` and watchdog `PingAPI`.
- Replaced JSON parsing with `parseProtoStats`, yielding a 10× parsing speedup (140 ns vs 1400 ns) and 2 allocations per batch.

---

## 2. End-to-End Streaming Migration to `encoding/json/v2`

- **Priority**: Medium
- **Status**: Backlog / Progressive Migration
- **Target Components**: [`internal/server/respond.go`](internal/server/respond.go), [`internal/sub/generate.go`](internal/sub/generate.go), [`web/src/api.ts`](web/src/api.ts)

### Background & Current State
- In Go 1.27.x, the standard library includes `encoding/json/v2` with `jsonv2.MarshalWrite` and `jsonv2.UnmarshalRead`.
- `writeOK` currently uses static pre-encoded JSON (`okJSON`), and `writeAPIPage` uses typed `PageEnvelope[T]`.
- Main API endpoints still use `json.NewEncoder(w).Encode(v)`.

### Proposed Architecture
- Replace `json.NewEncoder(w).Encode(v)` with streaming `jsonv2.MarshalWrite(w, v)`.
- Stream subscription configuration bodies (`GenerateSingBox`, `GenerateXrayJSON`) directly to `http.ResponseWriter` without buffering full multi-megabyte string payloads in memory.

### Trade-offs & Considerations
- **Behavioral Changes**: By default, `encoding/json/v2` marshals nil slices as `[]` (empty JSON array) rather than `null`.
- **Frontend Impact**: Frontend TypeScript interfaces (`web/src/api.ts`) and third-party API clients must be verified to ensure they do not strictly expect `null` on empty collections, or explicit `jsonv2.FormatNilSliceAsNull(true)` options must be supplied.

---

## 3. Automated Goroutine Leak Smoke Tests in CI

- **Priority**: Medium
- **Status**: Backlog
- **Target Components**: [`.github/workflows/`](.github/workflows/), [`internal/server/panel.go`](internal/server/panel.go)

### Background & Current State
- The `/api/debug/pprof/goroutineleak` endpoint is implemented and accessible in debug mode, tracking goroutine lifecycles and identifying hanging background loops.
- Verification is currently done manually via profiling tools.

### Proposed Architecture
- Add an automated CI test step in GitHub Actions:
  1. Boot the panel server in test mode.
  2. Run standard API load / subscription generation suite.
  3. Query `/api/debug/pprof/goroutineleak` before shutdown.
  4. Fail CI if any non-whitelisted goroutines remain active.

---

## 4. Toolchain Directive Pinning in `go.mod`

- **Priority**: Low / Maintenance
- **Status**: Ready
- **Target Components**: [`go.mod`](go.mod)

### Background & Current State
- `go.mod` declares `go 1.27.1`.
- Go 1.21+ supports the `toolchain` directive to guarantee that the Go toolchain automatically switches to the exact patch release when building.

### Proposed Action
- Add `toolchain go1.27.1` to `go.mod` to ensure uniform compiler behavior across developers and CI environments.
