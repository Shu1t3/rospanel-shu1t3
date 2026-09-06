---
description: Safely refactors a specific Go package using a verification loop and local tests.
---

When triggered, follow these sequential steps to refactor the target Go package:

1. **Analyze Dependencies**: Run `go list -m -f '{{.Deps}}' <target_package>` to understand the package context and external dependencies.
2. **Review & Plan**: Read all files in the target package. Based on `.agent/rules/go-idiomatic.md` and `.agent/rules/go-concurrency.md`, identify technical debt, anti-patterns, or concurrency issues. Write a concise refactoring plan and wait for my approval.
3. **Execute Refactoring**: Apply the approved plan to the files incrementally, file by file.
4. **Lint and Format Check**: Run `go fmt <target_package>/...` and `golangci-lint run <target_package>/...`. If the linter fails, automatically fix the reported errors.
5. **Run Tests**: Run `go test -v -race <target_package>/...`. If tests fail, analyze the git diff, locate the regression, and fix it.
