---
name: commit
description: Run validation checks and create a git commit. Invoke with /commit.
user_invocable: true
disable-model-invocation: true
---

# Commit Checklist

Run these checks before creating a git commit. Report issues and offer to fix them.

## 1. Formatting

**TypeScript/JS** (use `yarn` if `yarn.lock` exists): check `package.json` scripts for `biome` (run with `--write`), `format` script, or `prettier --write`. If `biome` is found, skip step 2 — biome handles both formatting and linting.

**Go**: `gofmt -w` on changed files.

## 2. Linting

**TypeScript/JS**: `yarn lint --fix` (skip if biome was used in step 1)

**Go**: `go vet ./...`

Stage any auto-fixed files from steps 1-2 before proceeding.

## 3. Review Changes

- `git diff --cached` — what will be committed
- `git diff` — flag important unstaged changes being left out

## 4. Scan for Problems

Scan staged files for:
- **Secrets**: API keys, tokens, passwords (`sk-`, `ghp_`, `AKIA`, `password =`, `secret =`), `.env` files, private keys. **Stop and warn if found.**
- **Debug code**: `console.log/debug/warn`, `debugger`, `fmt.Println`, `print(` — report and ask if intentional
- **TODO comments** added in this diff — OK if they reference a Linear ticket, otherwise ask if intentional
- **Commented-out code blocks**

## 5. Commit

- Write a short, punchy commit message — no conventional commit prefixes (`feat:`, `fix:`, etc.)
- Lowercase, no period at the end
- Summarize the *what*, not the *why*
- Create the commit
