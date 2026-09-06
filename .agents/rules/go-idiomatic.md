---
trigger: always_on
---

# Go Idiomatic Code & Refactoring Rules

## General Principles
- Enforce the rule: "Return concrete types, accept interfaces". Do not pollute the codebase with unnecessary interfaces if there is only one implementation.
- Strictly follow the Clean Architecture layers defined in this project. Never import a transport layer (HTTP/gRPC) or repository layer into the core domain/usecase packages.
- Keep functions small and focused on a single responsibility. If a function exceeds 50 lines, propose a refactoring to split it.

## Error Handling (Go 1.20+)
- Never discard errors using `_`. Always handle them or return them up the stack.
- When wrapping errors with additional context, use `fmt.Errorf("context message: %w", err)`.
- Use `errors.Is(err, target)` and `errors.As(err, &target)` for error checking instead of direct equality checks or type assertions.
- For aggregating multiple errors (e.g., in loops or parallel operations), strictly use `errors.Join()`.

## Naming & Style
- Avoid stuttering in package names and exported types (e.g., use `user.Service` instead of `user.UserService`).
- Run `go fmt` and `goimports` immediately after any code modification.
