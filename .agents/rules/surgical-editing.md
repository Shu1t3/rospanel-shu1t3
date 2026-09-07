# Surgical Code Edits

- **Do not refactor untouched code:** Only touch functions and lines directly related to the user prompt. Do not fix unrelated warnings, stylistic issues, or reorder imports unless requested.
- **Preserve formatting:** Avoid running full-file auto-formatters that create unnecessary diffs on lines you didn't touch.
- **No speculative code:** Do not generate helper functions, fallbacks, or test mocks "just in case" unless directly asked.
