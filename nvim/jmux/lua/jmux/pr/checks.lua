-- The latest commit's status checks as a pure model, shaped like
-- jmux.pr.reviewers: pv renders it as a collapsible block, a future
-- dashboard pane can render the same lines/marks.

local util = require "jmux.pr.util"

local nilable = util.nilable

local M = {}

-- check_mark maps a status-check node to (icon, highlight): green tick passed,
-- red cross failed, yellow dot running, muted circle skipped/neutral.
local function check_mark(node)
  local state = nilable(node.conclusion) or nilable(node.state)
  if not state then
    return "●", "DiagnosticWarn"
  end
  state = tostring(state):upper()
  if state == "SUCCESS" then
    return "✓", "DiagnosticOk"
  elseif state == "PENDING" or state == "EXPECTED" then
    return "●", "DiagnosticWarn"
  elseif state == "NEUTRAL" or state == "SKIPPED" or state == "CANCELLED" or state == "STALE" then
    return "○", "Comment"
  end
  return "✗", "DiagnosticError"
end

-- model distils the PR node's statusCheckRollup into { header, header_mark,
-- lines, marks }, or nil when the PR has no checks. The header summarises
-- ("1 failed" / "2 running" / "all passed") with the worst state's icon.
function M.model(pr)
  local checks = vim.tbl_get(pr, "commits", "nodes", 1, "commit", "statusCheckRollup", "contexts", "nodes")
  if type(checks) ~= "table" or #checks == 0 then
    return nil
  end
  local lines, marks, fail, run = {}, {}, 0, 0
  for _, c in ipairs(checks) do
    local icon, hl = check_mark(c)
    local name = c.name or c.context or "?"
    local dur = util.duration(c.startedAt, c.completedAt)
    lines[#lines + 1] = dur ~= "" and ("%s %s · %s"):format(icon, name, dur) or ("%s %s"):format(icon, name)
    marks[#marks + 1] = { rel = #lines - 1, col = 0, col_end = #icon, hl = hl }
    if hl == "DiagnosticError" then
      fail = fail + 1
    elseif hl == "DiagnosticWarn" then
      run = run + 1
    end
  end
  local summary = (fail > 0 and fail .. " failed") or (run > 0 and run .. " running") or "all passed"
  local icon, hl = "✓", "DiagnosticOk"
  if fail > 0 then
    icon, hl = "✗", "DiagnosticError"
  elseif run > 0 then
    icon, hl = "●", "DiagnosticWarn"
  end
  return {
    header = ("%s Checks · %s"):format(icon, summary),
    header_mark = { col = 0, col_end = #icon, hl = hl },
    lines = lines,
    marks = marks,
    running = run,
    failed = fail,
  }
end

return M
