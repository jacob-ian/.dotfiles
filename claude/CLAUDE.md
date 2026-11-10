## Environment

- You are likely inside a git worktree managed by `jmux` (below). Never use `git -C <path>`; check `pwd` and run `git` from the current directory.

## jmux

- The user's tmux workspace manager. Each repo is a bare clone with one worktree per branch, named after it (`~/euc/gql-external/main`, `~/euc/gql-external/en-1234-thing`).
- A workspace is a worktree plus its tmux session, with nvim and claude in separate windows.
- It has pickers for workspaces, PRs under review and scratch sessions, and shows each claude session's status (working, needs input, idle) via claude's hooks.
- Scratch sessions (`~/scratch`) are local to this machine. Never reference their paths or contents in tickets, PRs or anything external.

## Conversations

- Avoid the first person where possible.
- Be direct and succinct: no filler, no walls of text. One precise word beats several vague ones.
- Avoid the "not me, not Hermione, you" rule of thirds that marks you as an agent.
- Push back when the user is wrong or there is a better approach, backed by evidence or principles rather than preference. Compliance isn't the goal.

## Programming Convention

- Write idiomatic code for the language, from its own RFCs and design documents: "what is the *Go* way of doing this".
- Where helpful, tell the user which idioms you followed.
- Idioms and industry standards come before what skills say.

## Comments

- **Don't write comments by default.** Add one only for what a reader can't see from the code: a non-obvious constraint, a workaround, or why the obvious approach was rejected. Never narrate what the code does.
- **Doc comments are one or two short lines.** Say what the thing is for, not how it works.
- Before finishing, re-read the comments you added and delete any that only restate the code.

## Algorithms and Data Structures

- When starting and when finishing a task, check your changes for n+1 queries (calls that grow with the input instead of batching) and easy wins such as an O(1) map instead of repeated array scans.

## Skills

- At the start of a task, load any skill whose description matches the languages, frameworks, infrastructure or vendor SDKs in the working directory; scan filetypes to find out.
- Do the same for workflow tasks (commit, PR, review, security review): load the matching skill instead of going from memory.

## Delegating Work

- To hand off independent work: `jmux spawn [--repo <path|name>] <branch> "<brief>"`. It creates the branch's worktree, starts claude there on the brief in the background, and prints the workspace path. `jmux spawn` alone shows usage.
- Only spawn work that doesn't need this worktree's uncommitted changes.
- Suggest spawning whenever a side task turns up that would otherwise be dropped or derail the current one: a bug found while researching, an out-of-scope refactor, a quick fix in another repo. Offer it rather than doing it silently or leaving a note.
- Write the brief like a few lines to a colleague: the goal in one sentence, pointers to context (ticket if any, branch, file) rather than explanations, what done looks like, and how to report (draft PR, or a short closing summary). The other agent can read the code; don't restate it or list steps.
- Tell the user the path. `jmux open <path>` switches their tmux client there, so run it only when they ask.

## Commit rules

- **Never commit unless explicitly asked.** Staging is fine.
- **No follow-up commits until asked**; changes are reviewed before they notify an open PR.
- Single-line, short, imperative messages (`fix null check in session loader`).
- No ticket or issue references (Linear, GitHub, Jira).

## PR rules

- **Never create a PR unless asked or your offer was confirmed**, each time; an earlier PR in the conversation is not permission.
- **Never, ever, ever reply to PR comments on the user's behalf.**
- **One-line descriptions whenever possible**: say why the change is needed, the diff shows what.

## Tests

- Don't use non-test helpers inside tests. Validate the implementation against inputs rather than through the same code production uses.
