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
-- invariant that lets gh cache the PR identity. Memoizing it (false sentinel
-- caches a miss too) keeps the per-PR state files off a blocking `git rev-parse`
-- spawn on every read/write/clear, including the view's instant-paint open path.
local git_dir
function M.git_dir()
  if git_dir == nil then
    git_dir = M.sh { "git", "rev-parse", "--absolute-git-dir" } or false
  end
  return git_dir or nil
end

-- span_str renders a comment's line span as "12" or "12-15". Accepts both the
-- stored comment shape (start_line nil for a single line) and the target shape
-- (start_line always set, == line for a single line).
function M.span_str(cm)
  local s = cm.start_line
  return (s and s ~= cm.line) and string.format("%d-%d", s, cm.line) or tostring(cm.line)
end

-- The local UTC offset, computed once at load. os.time reads a broken-down table
-- as local time, so converting a UTC timestamp needs this constant added back.
-- Taken from "now", so a timestamp in the opposite DST period can be off by up to
-- an hour — fine for the coarse relative ages we render.
local utc_offset = os.difftime(os.time(os.date "*t"), os.time(os.date "!*t"))

-- iso_epoch converts an ISO-8601 UTC timestamp to a Unix epoch. os.time reads a
-- broken-down table as local time, so feeding it the UTC fields yields
-- epoch - offset; adding the offset back recovers the true UTC epoch.
local function iso_epoch(iso)
  local y, mo, d, h, mi, s = tostring(iso):match "(%d+)-(%d+)-(%d+)T(%d+):(%d+):(%d+)"
  if not y then
    return nil
  end
  return os.time {
    year = tonumber(y),
    month = tonumber(mo),
    day = tonumber(d),
    hour = tonumber(h),
    min = tonumber(mi),
    sec = tonumber(s),
  } + utc_offset
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
