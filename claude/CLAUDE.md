# Git usage

- You are likely running inside a git worktree. Never use `git -C <path>` — check `pwd` and run `git` from the current working directory.

# Commit rules

- **Never commit unless explicitly asked.** Staging files is fine, but do not run `git commit` until asked.
- Keep commit messages to a single line — short and imperative (e.g. `fix null check in session loader`).
- Do not include ticket or issue references (Linear, GitHub, Jira) in commit messages.
