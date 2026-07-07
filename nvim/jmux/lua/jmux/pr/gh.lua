-- GitHub access for the pr modules: one GraphQL entry point and the cached PR
-- identity. Works in a jmux PR worktree, where the checked-out branch is the
-- PR's head, so `gh` resolves the PR for every call with no number to pass.

local util = require "jmux.pr.util"

local M = {}

-- graphql runs one query/mutation with `vars` as GraphQL variables and calls
-- cb(data) with the decoded response body, or cb(nil, err) on failure — gh exits
-- non-zero when the response carries GraphQL errors, so both transport and query
-- failures land in err. The payload goes via --input as a raw JSON body, which
-- handles every variable type uniformly (nested arrays included) with no -f/-F
-- argument building. Async; cb runs on the main loop.
function M.graphql(query, vars, cb)
  local payload = vim.json.encode { query = query, variables = vars }
  vim.system({ "gh", "api", "graphql", "--input", "-" }, { stdin = payload, text = true }, function(res)
    vim.schedule(function()
      if res.code ~= 0 then
        local msg = res.stderr ~= "" and res.stderr or res.stdout
        cb(nil, vim.trim(msg or ""))
        return
      end
      local ok, data = pcall(vim.json.decode, res.stdout)
      if not ok then
        cb(nil, "unparseable GraphQL response")
        return
      end
      cb(data)
    end)
  end)
end

-- owner_repo splits an "owner/repo" slug for queries that take them separately.
function M.owner_repo(slug)
  return slug:match "([^/]+)/(.+)"
end

-- info caches the PR's { slug, number, base, id } from a single `gh pr view`.
-- Resolved lazily and cached for the whole session: one nvim instance maps to one
-- PR worktree (so the branch never changes under us), which is what keeps the
-- cached number/id valid.
local info

local INFO_FIELDS = "number,baseRefOid,url,id"

-- parse_pr_info turns `gh pr view --json` output into the { slug, number, base,
-- id } identity (id is the GraphQL node id, used by mutations). The slug comes
-- from the PR url path (host-agnostic, works on GHE); a parse failure falls back
-- to a repo lookup rather than caching a half-resolved identity.
local function parse_pr_info(out)
  local ok, data = pcall(vim.json.decode, out)
  if not ok or type(data) ~= "table" or not data.number then
    return nil, "gh pr view: unexpected output"
  end
  local slug = data.url and data.url:match "/([^/]+/[^/]+)/pull/"
  if not slug then
    local e
    slug, e = util.sh { "gh", "repo", "view", "--json", "nameWithOwner", "-q", ".nameWithOwner" }
    if not slug then
      return nil, "gh repo view: " .. (e or "")
    end
  end
  return { slug = slug, number = data.number, base = data.baseRefOid, id = type(data.id) == "string" and data.id or nil }
end

-- The identity also persists to the worktree's git dir (like the pending review
-- and the pv cache): a worktree maps to one PR for its lifetime, so slug/number/
-- id can never go stale, and a disk hit makes pd/pv open with zero network on a
-- fresh session. base (the base branch tip) does advance, so a disk hit still
-- refreshes in the background — see refresh_async.
local function info_file()
  local dir = util.git_dir()
  return dir and (dir .. "/jmux-pr-info.json") or nil
end

local function read_info_cache()
  local f = info_file()
  if not f or vim.fn.filereadable(f) == 0 then
    return nil
  end
  local ok, data = pcall(vim.json.decode, table.concat(vim.fn.readfile(f), "\n"))
  if not ok or type(data) ~= "table" or not data.number then
    return nil
  end
  return data
end

local function write_info_cache(i)
  local f = info_file()
  if f then
    vim.fn.writefile({ vim.json.encode(i) }, f)
  end
end

-- refresh_async re-resolves the identity off the main loop and rewrites both
-- caches, so a disk-cached open's base is at most one open behind the base
-- branch's tip (and a worktree whose branch grew a new PR self-heals the same
-- way). Once per session — the identity doesn't drift faster than that matters.
local refreshed = false
local function refresh_async()
  if refreshed then
    return
  end
  refreshed = true
  vim.system({ "gh", "pr", "view", "--json", INFO_FIELDS }, { text = true }, function(res)
    vim.schedule(function()
      if res.code ~= 0 then
        return
      end
      local i = parse_pr_info(res.stdout)
      if i then
        info = i
        write_info_cache(i)
      end
    end)
  end)
end

-- cached returns the session or disk copy of the identity, kicking the
-- background refresh on a disk hit. nil when neither exists yet.
local function cached()
  if info then
    return info
  end
  info = read_info_cache()
  if info then
    refresh_async()
  end
  return info
end

-- pr_info resolves the PR identity: the session cache, then the disk cache
-- (kicking a background refresh), then one blocking gh round trip — so only the
-- very first open of a worktree ever waits on the network. Callers that must not
-- freeze the editor even then use pr_info_async.
function M.pr_info()
  local i = cached()
  if i then
    return i
  end
  local out, err = util.sh { "gh", "pr", "view", "--json", INFO_FIELDS }
  if not out then
    return nil, "gh pr view: " .. (err or "")
  end
  local perr
  i, perr = parse_pr_info(out)
  info = i
  if i then
    write_info_cache(i)
  end
  return i, perr
end

-- pr_info_async resolves the identity off the main loop: the session or disk
-- cache immediately when present, else via vim.system so the editor stays
-- responsive (e.g. an animated loader keeps spinning) during the round trip.
function M.pr_info_async(cb)
  local i = cached()
  if i then
    cb(i)
    return
  end
  vim.system({ "gh", "pr", "view", "--json", INFO_FIELDS }, { text = true }, function(res)
    vim.schedule(function()
      if res.code ~= 0 then
        cb(nil, "gh pr view: " .. vim.trim(res.stderr or ""))
        return
      end
      local perr
      i, perr = parse_pr_info(res.stdout)
      info = i
      if i then
        write_info_cache(i)
      end
      cb(i, perr)
    end)
  end)
end

return M
