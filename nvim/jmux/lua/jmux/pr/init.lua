-- Lightweight GitHub PR review. Stage inline comments from diffview selections
-- into a pending review, then submit once with a verdict; replies to existing
-- threads post immediately. Replaces octo for the review flow. Works in a jmux
-- PR worktree, where the checked-out branch is the PR's head, so `gh` resolves
-- the PR for every call with no number to pass.
--
-- The implementation is layered under pr/: util → gh → ui/state → diff →
-- review/view. This facade re-exports the public surface the keymaps bind.

local diff = require "jmux.pr.diff"
local review = require "jmux.pr.review"
local view = require "jmux.pr.view"

return {
  add_comment = diff.add_comment,
  suggest = diff.suggest,
  diff = diff.diff,
  review = review.review,
  discard = review.discard,
  view = view.view,
  browser = view.browser,
}
