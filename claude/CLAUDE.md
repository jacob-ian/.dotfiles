# Skills

- Before starting work in a repo, scan the available skills and proactively load any whose description matches the languages, frameworks, infrastructure, or vendor SDKs visible in the working directory (e.g. a Go skill for `.go` files, a SQL skill for migrations, a Terraform skill for `.tf`, a Stripe skill if the Stripe SDK is imported).
- The same applies to workflow tasks: before writing a commit message, opening a PR, doing a code review, etc., check for and load any matching skill (commit, PR, review, security-review, etc.) instead of going from memory.
- Load skills at the start of the task, not after the user asks.

# Git usage

- You are likely running inside a git worktree. Never use `git -C <path>` — check `pwd` and run `git` from the current working directory.

# Commit rules

- **Never commit unless explicitly asked.** Staging files is fine, but do not run `git commit` until asked.
- **Do not create follow up commits until asked**. I need to first review your changes before it notifies an open PR.
- Keep commit messages to a single line — short and imperative (e.g. `fix null check in session loader`).
- Do not include ticket or issue references (Linear, GitHub, Jira) in commit messages.

# PR rules

- **Never create a PR unless the user asked or has confirmed your offer**. Even if you have previously created a PR in the context window, subsequent PRs MUST be permitted by the user only.
- **Never, ever, ever reply on my behalf to PR comments**.
- **Use single line PR descriptions whenever possible**. PR descriptions should explain why the change is needed, the diff explains what the change is. This can typically be one line.
