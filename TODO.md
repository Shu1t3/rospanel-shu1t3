# RosPanel: Architectural Roadmap & Large Changes (TODO)

This document tracks large-scale architectural and performance enhancements identified during the **Go 1.27.x** audit that involve dependency additions, protocol changes, or breaking contract considerations.

---

## 1. Direct In-Process RPC Client for Xray StatsService

- **Priority**: High (Architecture)
- **Status**: Backlog / Future Phase
- **Target Components**: [`internal/xray/supervisor.go`](internal/xray/supervisor.go), [`internal/core/manager_stats.go`](internal/core/manager_stats.go)

### Background & Current State
- `Supervisor.QueryStats(apiAddr)` currently invokes `exec.CommandContext` to run the `xray api statsquery` CLI binary.
- A 1.5s coalescing cache (`statsMu`, `lastStatsTime`, `lastStatsVal`) is in place to coalesce overlapping queries between the 3s dashboard speed loop (`vpnSpeedLoop`), 2s status feed (`statusFeed`), and admin panel requests.
- While concurrent forks are eliminated, periodic query execution still spawns a new process when the cache expires.

### Proposed Architecture
- Implement an in-process gRPC client connecting directly to Xray's API port (`127.0.0.1:<api_port>`).
- Query `StatsService/QueryStats` over a persistent TCP or Unix Domain Socket connection using stream/unary RPC.

### Trade-offs & Considerations
- **Pros**: Completely eliminates `fork()` and `execve()` overhead; reduces stats query latency from ~15–30 ms to <1 ms; saves 15–20% CPU on 1-vCPU VPS instances while the dashboard is open.
- **Cons / Costs**: Requires importing `google.golang.org/grpc`, `google.golang.org/protobuf`, and Xray's protobuf stubs into `go.mod`, increasing dependency tree and binary size.
- **Alternative**: Maintain minimal pure-Go wire client over raw HTTP/gRPC frame without pulling the full Google gRPC stack.

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
