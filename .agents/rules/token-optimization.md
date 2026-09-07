# Token Optimization & Project Analysis Rules

## 1. Context Boundaries & File Reading
- **Never auto-read entire codebases** at session start. Load context lazily, strictly based on the user's specific request.
- **Ignore heavy and generated files:** Never inspect or parse:
  - Dependency/lock files (`package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `Cargo.lock`, `poetry.lock`, `composer.lock`).
  - Build/cache directories (`dist/`, `build/`, `.next/`, `coverage/`, `.turbo/`, `target/`, `node_modules/`, `venv/`).
  - Media, binaries, minified files (`*.min.js`, `*.bundle.*`), and database dumps (`*.sql`, `*.sqlite`).
- **Signature-First Inspection:** When exploring unfamiliar modules, inspect only interfaces, type definitions, function signatures, or top-level comments. Do not read complete file implementations unless direct changes are required.
- **Partial Reads:** For files longer than 150 lines, read only relevant line ranges rather than the full file.

## 2. Search & Exploration Behavior
- Use targeted symbol/regex search over broad recursive directory listings.
- When listing directory contents, restrict depth to a maximum of 2 levels unless explicitly instructed.
- Do not dump directory trees or lists of files into the chat.

## 3. Response Density & Output
- **No Echoing:** Never repeat entire existing files or large boilerplate blocks. Provide only unified diffs or minimal code snippets highlighting modified lines.
- **Minimal Conversational Overhead:** Skip pleasantries, introductions, and generic summaries at the end of responses. Start directly with the answer or action.
- Assume full context from previous turns without restating the problem definition.
