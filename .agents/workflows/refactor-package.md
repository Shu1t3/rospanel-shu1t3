---
description: Safely refactors a specific Go package using a verification loop and local tests.
---

When triggered, follow these sequential steps to refactor the target Go package:

1. **Analyze Dependencies**: Run `go list -m -f '{{.Deps}}' <target_package>` to understand the package context and external dependencies.
2. **Review & Plan**: Read the target package files. Based on `.agents/rules/go-idiomatic.md` and `.agents/rules/go-concurrency.md`, identify technical debt, anti-patterns, or concurrency issues. Formulate a concise refactoring plan (3–5 bullets). If changes break exported public APIs, request user confirmation; otherwise, proceed directly to execution.
3. **Execute Refactoring**: Apply the plan incrementally, file by file.
4. **Lint and Format Check**: Run `go fmt ./<target_package>/...` and `golangci-lint run ./<target_package>/...`. If the linter fails, fix the reported errors.
5. **Run Tests**: Run `go test -v -race ./<target_package>/...`. If tests fail, analyze the git diff, locate the regression, and fix it.
