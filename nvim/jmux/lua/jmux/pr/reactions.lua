-- GitHub's reaction vocabulary and queries, shared by the conversation model
-- (rendering counts) and pv's interactions (the + picker and K hover).

local util = require "jmux.pr.util"

local nilable = util.nilable

local M = {}

---@alias jmux.pr.ReactionGroup { content: string, viewerHasReacted: boolean?, users: { totalCount: integer, nodes: { login: string? }[]? } }
---@alias jmux.pr.Reactable { reactionGroups: jmux.pr.ReactionGroup[]? }

-- FIELDS is the reactionGroups selection a reactable carries in the main
-- query: counts only — bare totalCounts cost nothing against GitHub's 500k
-- node limit, while logins under threads × comments × users multiply past
-- it. Logins come from REACTORS_QUERY on demand.
M.FIELDS = "reactionGroups{ content viewerHasReacted users{ totalCount } }"

-- REACTORS_QUERY resolves who reacted, on demand for the K hover.
M.REACTORS_QUERY = [[
query($ids:[ID!]!){
  nodes(ids:$ids){
    id
    ... on Reactable{ reactionGroups{ content users(first:100){ totalCount nodes{ login } } } }
  }
}]]

-- ADD_MUTATION / REMOVE_MUTATION set and unset the viewer's reaction on any
-- Reactable node, for the + react picker.
M.ADD_MUTATION = [[
mutation($subject:ID!,$content:ReactionContent!){
  addReaction(input:{subjectId:$subject,content:$content}){ clientMutationId }
}]]
M.REMOVE_MUTATION = [[
mutation($subject:ID!,$content:ReactionContent!){
  removeReaction(input:{subjectId:$subject,content:$content}){ clientMutationId }
}]]

-- EMOJI maps GitHub's ReactionContent enum to the emoji GitHub renders;
-- ORDER fixes the react picker's listing to GitHub's own.
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
M.ORDER = { "THUMBS_UP", "THUMBS_DOWN", "LAUGH", "HOORAY", "CONFUSED", "HEART", "ROCKET", "EYES" }

-- line renders a node's reactions as "👍 2 · 🎉 1", or nil when nothing has
-- reacted (including nodes fetched before reactions were queried).
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

-- mine collects the reaction contents the viewer has already given a node,
-- so the react picker offers those as removals.
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

-- who lists a node's reactions with their reactors, one line per emoji
-- ("👍 @alice @bob"), or nil when nothing has reacted. Reactors beyond the
-- reactor query's page collapse to "+n more".
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
