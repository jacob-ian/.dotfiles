-- GitHub access for the pr modules: one GraphQL entry point and the cached PR
-- identity.

local util = require "jmux.pr.util"

local M = {}

-- graphql runs one query/mutation async and calls cb(data) or cb(nil, err) on
-- the main loop. gh exits non-zero on GraphQL errors too, so both land in err.
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

-- info is the PR's { slug, number, base, id }, cached for the session: one nvim
-- maps to one PR worktree, so it never changes.
local info

local INFO_FIELDS = "number,baseRefOid,baseRefName,url,id"

-- parse_pr_info turns `gh pr view --json` output into the identity. The slug
-- comes from the PR url so it works on GHE.
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
  return {
    slug = slug,
    number = data.number,
    base = data.baseRefOid,
    baseRef = type(data.baseRefName) == "string" and data.baseRefName or nil,
    id = type(data.id) == "string" and data.id or nil,
  }
end

-- The identity also persists in the git dir so a fresh session opens with no
-- network. base advances, so a disk hit still refreshes in the background.
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

-- pr_info resolves the PR identity, blocking on gh only when no cache exists.
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

-- pr_info_async is pr_info off the main loop, for callers animating a loader.
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
