# Terminal & Command Discipline

- **Mute verbose outputs:** Always append quiet or minimal flags to CLI commands (e.g., `npm test -- -q`, `pytest -q`, `curl -s`).
- **Truncate logs:** If running commands with potentially large output, pipe or limit them (e.g., pipe through `head -n 30`, `tail -n 30`, or `grep`).
- **Targeted testing:** Never run full test suites or global linters automatically. Run strictly the single unit test or file relevant to the immediate change.
- Never run production build scripts (`build`, `bundle`, `compile`) unless explicitly asked to verify the build.
