---
description: Generates high-coverage table-driven unit tests for a specific Go file.
---

When triggered, follow these sequential steps to generate tests for the specified Go file:

1. **Analyze Code**: Analyze the target Go file. Identify all logical branches, edge cases, happy paths, and potential error scenarios.
2. **Generate Test File**: Create or update the corresponding `_test.go` file. 
   - Strictly use the standard Go table-driven test pattern (`struct` with test cases).
   - Use `t.Parallel()` for parallel test execution where appropriate.
   - Ensure you cover at least 3 edge cases or error scenarios for each public function.
3. **Verify & Run**: Run `go test -v -cover` for the modified package to verify that the generated tests compile, pass successfully, and increase test coverage. If tests fail, analyze the compilation or runtime error and fix the test code.
