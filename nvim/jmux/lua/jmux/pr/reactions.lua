-- GitHub's reaction vocabulary and queries.

local util = require "jmux.pr.util"

local nilable = util.nilable

local M = {}

---@alias jmux.pr.ReactionGroup { content: string, viewerHasReacted: boolean?, users: { totalCount: integer, nodes: { login: string? }[]? } }
---@alias jmux.pr.Reactable { reactionGroups: jmux.pr.ReactionGroup[]? }

-- Counts only: logins under threads × comments × users blow past GitHub's
-- 500k node limit, so REACTORS_QUERY fetches them on demand.
M.FIELDS = "reactionGroups{ content viewerHasReacted users{ totalCount } }"

M.REACTORS_QUERY = [[
query($ids:[ID!]!){
  nodes(ids:$ids){
    id
    ... on Reactable{ reactionGroups{ content users(first:100){ totalCount nodes{ login } } } }
  }
}]]

M.ADD_MUTATION = [[
mutation($subject:ID!,$content:ReactionContent!){
  addReaction(input:{subjectId:$subject,content:$content}){ clientMutationId }
}]]
M.REMOVE_MUTATION = [[
mutation($subject:ID!,$content:ReactionContent!){
  removeReaction(input:{subjectId:$subject,content:$content}){ clientMutationId }
}]]

M.EMOJI = {
  THUMBS_UP = "👍",
  THUMBS_DOWN = "👎",
  LAUGH = "😄",
  HOORAY = "🎉",
  CONFUSED = "😕",
  HEART = "❤️",
  ROCKET = "🚀",
  EYES = "👀",
}
-- ORDER is GitHub's own listing order.
M.ORDER = { "THUMBS_UP", "THUMBS_DOWN", "LAUGH", "HOORAY", "CONFUSED", "HEART", "ROCKET", "EYES" }

-- line renders a node's reactions as "👍 2 · 🎉 1", or nil when none.
---@param n jmux.pr.Reactable
---@return string?
function M.line(n)
  local parts = {}
  for _, g in ipairs(nilable(n.reactionGroups) or {}) do
    local count = vim.tbl_get(g, "users", "totalCount") or 0
    if count > 0 then
      parts[#parts + 1] = ("%s %d"):format(M.EMOJI[g.content] or g.content, count)
    end
  end
  if #parts > 0 then
    return table.concat(parts, " · ")
  end
end

-- mine is the set of reaction contents the viewer has given n.
---@param n jmux.pr.Reactable
---@return table<string, boolean>
function M.mine(n)
  local mine = {}
  for _, g in ipairs(nilable(n.reactionGroups) or {}) do
    if g.viewerHasReacted then
      mine[g.content] = true
    end
  end
  return mine
end

-- who lists reactions with their reactors, one line per emoji, or nil when
-- none; reactors beyond the fetched page collapse to "+n more".
---@param n jmux.pr.Reactable
---@return string[]?
function M.who(n)
  local out = {}
  for _, g in ipairs(nilable(n.reactionGroups) or {}) do
    local total = vim.tbl_get(g, "users", "totalCount") or 0
    if total > 0 then
      local names = {}
      for _, u in ipairs(vim.tbl_get(g, "users", "nodes") or {}) do
        names[#names + 1] = "@" .. (u.login or "?")
      end
      if total > #names then
        names[#names + 1] = ("+%d more"):format(total - #names)
      end
      out[#out + 1] = ("%s %s"):format(M.EMOJI[g.content] or g.content, table.concat(names, " "))
    end
  end
  if #out > 0 then
    return out
  end
end

return M
