-- GitHub PR review: stage inline comments from diffview into a pending review
-- and submit once with a verdict. Runs in a jmux PR worktree, so `gh` resolves
-- the PR from the checked-out branch.

local agent = require "jmux.pr.agent"
local diff = require "jmux.pr.diff"
local review = require "jmux.pr.review"
local view = require "jmux.pr.view"

return {
  add_comment = diff.add_comment,
  suggest = diff.suggest,
  diff = diff.diff,
  review = review.review,
  view = view.view,
  browser = view.browser,
  agent_review = agent.review,
  agent_send = agent.send,
}
