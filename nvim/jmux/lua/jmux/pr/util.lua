-- Shared plumbing for the pr modules: shell, JSON-null handling, git dir
-- resolution, and the compact time formatting used across the views.

local M = {}

function M.sh(cmd)
  local out = vim.fn.system(cmd)
  if vim.v.shell_error ~= 0 then
    return nil, vim.trim(out)
  end
  return vim.trim(out), nil
end

-- nilable collapses vim.NIL (JSON null) to nil so `or`/truthiness work.
function M.nilable(v)
  if v == nil or v == vim.NIL then
    return nil
  end
  return v
end

-- git_dir resolves the worktree's absolute git dir once per session. One nvim
-- instance maps to one PR worktree, so the path never changes under us — the same
-- invariant that lets gh cache the PR identity. Memoizing it keeps the per-PR
-- state files off a blocking `git rev-parse` spawn on every read/write/clear,
-- including the view's instant-paint open path. Only a hit is memoized: a miss
-- can heal (git init, cd) within the session.
local git_dir
function M.git_dir()
  if not git_dir then
    git_dir = M.sh { "git", "rev-parse", "--absolute-git-dir" }
  end
  return git_dir
end

-- prompt_remove_workspace offers to remove the current workspace (worktree +
-- tmux session) via a detached `jmux workspace remove` — the session kill takes
-- this nvim down with it, so on "yes" nothing after the call runs. on_keep runs
-- when the user declines or cancels.
function M.prompt_remove_workspace(on_keep)
  local keep = on_keep or function() end
  local root = M.sh { "git", "rev-parse", "--show-toplevel" }
  if not root then
    return keep()
  end
  local prompt = "clean up this workspace?"
  if (M.sh { "git", "status", "--porcelain" } or "") ~= "" then
    prompt = "clean up this workspace? (has uncommitted changes)"
  end
  require("jmux.pr.ui").confirm(prompt, function()
    vim.system({ "jmux", "workspace", "remove", "--path", root, "--quiet" }, { detach = true })
  end, keep)
end

-- span_str renders a comment's line span as "12" or "12-15". Accepts both the
-- stored comment shape (start_line nil for a single line) and the target shape
-- (start_line always set, == line for a single line).
function M.span_str(cm)
  local s = cm.start_line
  return (s and s ~= cm.line) and string.format("%d-%d", s, cm.line) or tostring(cm.line)
end

-- days_from_civil counts days from 1970-01-01 to a proleptic Gregorian date
-- (Howard Hinnant's algorithm). Shifts the year to start in March so leap days
-- fall at the end and the month-length table collapses to a linear formula.
local function days_from_civil(y, m, d)
  if m <= 2 then
    y = y - 1
  end
  local era = math.floor(y / 400)
  local yoe = y - era * 400
  local mp = (m + 9) % 12
  local doy = math.floor((153 * mp + 2) / 5) + d - 1
  local doe = yoe * 365 + math.floor(yoe / 4) - math.floor(yoe / 100) + doy
  return era * 146097 + doe - 719468
end

-- iso_epoch converts an ISO-8601 UTC timestamp to a Unix epoch. Lua has no
-- timegm and os.time reads a broken-down table as local time, with DST applied
-- per-date, so going through it leaks the local zone into the result. Pure
-- civil-date arithmetic sidesteps the zone entirely.
local function iso_epoch(iso)
  local y, mo, d, h, mi, s = tostring(iso):match "(%d+)-(%d+)-(%d+)T(%d+):(%d+):(%d+)"
  if not y then
    return nil
  end
  local days = days_from_civil(tonumber(y), tonumber(mo), tonumber(d))
  return days * 86400 + tonumber(h) * 3600 + tonumber(mi) * 60 + tonumber(s)
end

-- short_age renders a positive seconds delta as "now/5m/3h/2d/4mo".
function M.short_age(diff)
  if diff < 60 then
    return "now"
  elseif diff < 3600 then
    return math.floor(diff / 60) .. "m"
  elseif diff < 86400 then
    return math.floor(diff / 3600) .. "h"
  elseif diff < 2592000 then
    return math.floor(diff / 86400) .. "d"
  end
  return math.floor(diff / 2592000) .. "mo"
end

-- reltime renders a timestamp as a short age: "now/5m/3h/2d/4mo".
function M.reltime(iso)
  local at = iso_epoch(iso)
  if not at then
    return ""
  end
  return M.short_age(os.time() - at)
end

-- duration renders the elapsed time between two timestamps as "45s/2m5s/1h3m".
function M.duration(from, to)
  local a, b = iso_epoch(from), iso_epoch(to)
  if not a or not b or b < a then
    return ""
  end
  local d = b - a
  if d < 60 then
    return d .. "s"
  elseif d < 3600 then
    local m, s = math.floor(d / 60), d % 60
    return s > 0 and (m .. "m" .. s .. "s") or (m .. "m")
  end
  local h, m = math.floor(d / 3600), math.floor((d % 3600) / 60)
  return m > 0 and (h .. "h" .. m .. "m") or (h .. "h")
end

return M
