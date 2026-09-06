---
trigger: always_on
---

# Go Concurrency, Memory & Performance Rules

## Concurrency & Goroutines
- Every goroutine spawned MUST have a deterministic lifecycle. Always ensure it can be gracefully stopped using `context.Context` or explicit close channels.
- Prevent goroutine leaks: when using `time.Ticker` or `time.Timer`, always ensure `defer ticker.Stop()` is called.
- Avoid shared state where possible. Use channels for communication instead of heavy `sync.Mutex` locking, unless it's a simple thread-safe map/counter.

## Memory Optimization & Performance
- In high-throughput paths (e.g., DB queries, loops), avoid unnecessary allocations. 
- When initializing slices or maps with a known target size, always pre-allocate capacity: use `make([]T, 0, capacity)` instead of `make([]T, 0)`.
- Pay attention to escape analysis: do not return pointers to short-lived local variables from hot paths if they can be allocated on the stack.
