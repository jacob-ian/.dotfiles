-- Merge readiness as a pure model: the byline summary chip and the fields
-- pv's m/M keys decide with. Same pane-ready shape as the other models.

local util = require "jmux.pr.util"

local nilable = util.nilable

local M = {}

-- FIELDS is the PR selection this model reads.
M.FIELDS = [[isDraft mergeable reviewDecision mergeStateStatus
      autoMergeRequest{ enabledAt }]]

-- allowlist the repo's default merge method to a literal gh flag, so crafted
-- API data can never inject an arbitrary argument.
local FLAGS = { merge = "--merge", squash = "--squash", rebase = "--rebase" }

-- summary distils review + merge state into a one-line readiness chip
-- { icon, hl, text } for an open PR, or nil for merged/closed cases where
-- the status line already says it all.
---@return { icon: string, hl: string, text: string }?
function M.summary(pr)
  if tostring(pr.state or ""):upper() ~= "OPEN" then
    return nil
  end
  -- unresolved threads block the merge when the branch requires resolution; that
  -- rule isn't readable without push access, so surface the count regardless.
  local unresolved = 0
  for _, t in ipairs(vim.tbl_get(pr, "reviewThreads", "nodes") or {}) do
    if not t.isResolved then
      unresolved = unresolved + 1
    end
  end
  local mss = tostring(nilable(pr.mergeStateStatus) or ""):upper()
  local decision = tostring(nilable(pr.reviewDecision) or ""):upper()
  local mergeable = tostring(nilable(pr.mergeable) or ""):upper()
  local icon, hl, state_str = "●", "DiagnosticWarn", "pending"
  if pr.isDraft then
    icon, hl, state_str = "○", "Comment", "draft"
  elseif mergeable == "CONFLICTING" or mss == "DIRTY" then
    icon, hl, state_str = "✗", "DiagnosticError", "conflicts"
  elseif decision == "CHANGES_REQUESTED" then
    icon, hl, state_str = "✗", "DiagnosticError", "changes requested"
  elseif mss == "CLEAN" then
    icon, hl, state_str = "✓", "DiagnosticOk", "ready to merge"
  elseif mss == "BEHIND" then
    icon, hl, state_str = "●", "DiagnosticWarn", "behind base"
  elseif mss == "UNSTABLE" then
    icon, hl, state_str = "●", "DiagnosticWarn", "checks failing"
  elseif decision == "REVIEW_REQUIRED" then
    icon, hl, state_str = "●", "DiagnosticWarn", "review required"
  elseif mss == "BLOCKED" then
    icon, hl, state_str = "●", "DiagnosticWarn", "blocked"
  elseif decision == "APPROVED" then
    icon, hl, state_str = "✓", "DiagnosticOk", "approved"
  end
  -- approval counts live in the reviewers pane, so the summary carries only
  -- what that pane can't: thread resolution and merge state.
  local parts = {}
  if unresolved > 0 then
    parts[#parts + 1] = unresolved .. " unresolved"
  end
  parts[#parts + 1] = state_str
  if nilable(pr.autoMergeRequest) then
    parts[#parts + 1] = "auto-merge armed"
  end
  return { icon = icon, hl = hl, text = table.concat(parts, " · ") }
end

-- model resolves what the merge keys need: how/flag from the repo's default
-- method (nil flag = no safe method to run), whether GitHub reports the PR
-- ready (nothing left to wait for), and whether auto-merge is already armed.
---@param method string? the repository's viewerDefaultMergeMethod
---@return { how: string?, flag: string?, ready: boolean, armed: boolean }
function M.model(pr, method)
  local how = method and tostring(method):lower()
  return {
    how = how,
    flag = how and FLAGS[how],
    ready = tostring(nilable(pr.mergeStateStatus) or ""):upper() == "CLEAN",
    armed = nilable(pr.autoMergeRequest) ~= nil,
  }
end

return M
