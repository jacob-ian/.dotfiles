-- The conversation as a pure model, same pane-ready shape as
-- jmux.pr.reviewers/checks: header lines (title/byline with the merge
-- summary), the description block, and the time-ordered blocks of comments,
-- reviews and inline threads — plus the row-intent maps (reactions, subjects,
-- jumps) a renderer keys actions off.

local merge = require "jmux.pr.merge"
local reactions = require "jmux.pr.reactions"
local util = require "jmux.pr.util"

local nilable = util.nilable
local reltime = util.reltime

local M = {}

-- The conversation's selections in the main query. PR_FIELDS covers the node
-- scalars and description; the three connection field lists are also what
-- view's pagination re-requests per page.
M.PR_FIELDS = ("id title number state createdAt author{login} body %s"):format(reactions.FIELDS)
M.COMMENT_FIELDS = ("id author{login} body createdAt %s"):format(reactions.FIELDS)
M.REVIEW_FIELDS = ("id author{login} body state createdAt %s"):format(reactions.FIELDS)
M.THREAD_FIELDS = ("id isResolved isOutdated path line comments(first:100){ nodes{ id author{login} body createdAt pullRequestReview{id} %s } }"):format(
  reactions.FIELDS
)

-- model turns the PR node into { header, header_marks, pr, blocks, reactions,
-- subjects, jumps }. Each block is { id, header, body, kids, body_marks? }; a
-- review's inline threads are its kids (each kid { id, header, lines }).
-- reactions backs the K hover (reactable node ids, fetched reactor lines
-- memoize onto entries), subjects backs the + picker (a thread reacts to its
-- root comment), and jumps backs <CR> (the code a thread comments on).
function M.model(pr)
  local function who(n)
    return vim.tbl_get(n, "author", "login") or "?"
  end
  -- GitHub returns bodies with CRLF endings; strip the \r so it doesn't render
  -- as a trailing ^M on every line.
  local function split(text)
    return vim.split(vim.trim((text or ""):gsub("\r", "")), "\n")
  end

  local status = ("`%s` · @%s · %s"):format(tostring(pr.state or ""):lower(), who(pr), reltime(pr.createdAt or ""))
  local header_marks = {}
  local chip = merge.summary(pr)
  if chip then
    local prefix = status .. " · "
    status = ("%s%s %s"):format(prefix, chip.icon, chip.text)
    header_marks[#header_marks + 1] = { row = 1, col = #prefix, col_end = #prefix + #chip.icon, hl = chip.hl }
  end
  local header = {
    ("# %s  #%s"):format(pr.title or "", tostring(pr.number or "")),
    status,
    "",
  }

  local rx_by_id, subjects, jumps = {}, {}, {}

  -- the PR description is its own collapsible block, above the conversation.
  local pr_header = ("@%s · opened · %s"):format(who(pr), reltime(pr.createdAt or ""))
  local pr_rx = reactions.line(pr)
  if pr_rx then
    pr_header = pr_header .. " · " .. pr_rx
  end
  if nilable(pr.id) then
    subjects.pr = { id = pr.id, mine = reactions.mine(pr) }
    if pr_rx then
      rx_by_id.pr = { items = { { id = pr.id } } }
    end
  end
  local pr_block = {
    id = "pr",
    header = pr_header,
    body = split(pr.body),
    kids = {},
  }

  -- group inline threads under the review that opened them.
  local by_review, orphans = {}, {}
  for _, t in ipairs(vim.tbl_get(pr, "reviewThreads", "nodes") or {}) do
    local rid = vim.tbl_get(t, "comments", "nodes", 1, "pullRequestReview", "id")
    if rid then
      by_review[rid] = by_review[rid] or {}
      table.insert(by_review[rid], t)
    else
      orphans[#orphans + 1] = t
    end
  end
  local function thread_kid(t)
    local loc = t.path or "?"
    if nilable(t.line) then
      loc = loc .. ":" .. tostring(t.line)
    end
    local comments = vim.tbl_get(t, "comments", "nodes") or {}
    -- header: author/time · status; the location sits beneath it, then the body
    -- (replies marked ↳).
    local status_str = t.isResolved and "✓ resolved" or "● open"
    if t.isOutdated then
      status_str = status_str .. " · outdated"
    end
    local lines = { ("`%s`"):format(loc) }
    -- the root's reactions ride on the kid's header; replies get a bold
    -- author line (line_marks) carrying theirs the same way. line_subjects
    -- maps each line to its comment, so + reacts to the message under the
    -- cursor rather than always the root.
    local line_marks, line_subjects = {}, {}
    for i, c in ipairs(comments) do
      local first = #lines + 1
      if i > 1 then
        local hdr = ("↳ @%s · %s"):format(who(c), reltime(c.createdAt or ""))
        line_marks[#line_marks + 1] = { rel = #lines, col = 0, col_end = #hdr, hl = "Title" }
        local rx = reactions.line(c)
        if rx then
          hdr = hdr .. " · " .. rx
        end
        lines[#lines + 1] = hdr
      end
      vim.list_extend(lines, split(c.body))
      lines[#lines + 1] = ""
      if nilable(c.id) then
        local subj = { id = c.id, mine = reactions.mine(c) }
        -- excluding the trailing blank keeps the separator neutral ground.
        for l = first, #lines - 1 do
          line_subjects[l] = subj
        end
        -- reacted messages are K-hoverable individually, keyed by comment id
        -- (the thread's own id keys the aggregated hover).
        if reactions.line(c) then
          rx_by_id[c.id] = { items = { { id = c.id } } }
        end
      end
    end
    local root = comments[1]
    local header_str = root and ("@%s · %s · %s"):format(who(root), reltime(root.createdAt or ""), status_str)
      or (loc .. " · " .. status_str)
    local rx = root and reactions.line(root)
    if rx then
      header_str = header_str .. " · " .. rx
    end
    -- the whole thread's reactors hover on the kid's row, replies labelled by
    -- author so their reactions stay attributable.
    local items = {}
    for i, c in ipairs(comments) do
      if reactions.line(c) and nilable(c.id) then
        items[#items + 1] = { id = c.id, prefix = i > 1 and ("↳ @%s"):format(who(c)) or nil }
      end
    end
    if #items > 0 then
      rx_by_id[t.id or loc] = { items = items }
    end
    if root and nilable(root.id) then
      subjects[t.id or loc] = { id = root.id, mine = reactions.mine(root) }
    end
    if nilable(t.path) then
      jumps[t.id or loc] = { path = t.path, line = nilable(t.line) }
    end
    return {
      id = t.id or loc,
      header = header_str,
      lines = lines,
      line_marks = line_marks,
      line_subjects = line_subjects,
    }
  end

  -- time-ordered conversation (ISO-8601 sorts chronologically as a string).
  local blocks = {}
  for _, c in ipairs(vim.tbl_get(pr, "comments", "nodes") or {}) do
    local header_str = ("@%s · %s"):format(who(c), reltime(c.createdAt or ""))
    local id = "c" .. tostring(c.createdAt) .. who(c)
    local rx = reactions.line(c)
    if nilable(c.id) then
      subjects[id] = { id = c.id, mine = reactions.mine(c) }
      if rx then
        rx_by_id[id] = { items = { { id = c.id } } }
      end
    end
    if rx then
      header_str = header_str .. " · " .. rx
    end
    blocks[#blocks + 1] = {
      at = c.createdAt or "",
      id = id,
      header = header_str,
      body = split(c.body),
      kids = {},
    }
  end
  for _, r in ipairs(vim.tbl_get(pr, "reviews", "nodes") or {}) do
    local rt = (r.id and by_review[r.id]) or {}
    local verdict = tostring(r.state or ""):upper()
    -- show a review when it carries a body, inline threads, or a verdict — the
    -- last so a bare "Approve" / "Request changes" still shows who weighed in.
    if vim.trim(r.body or "") ~= "" or #rt > 0 or verdict == "APPROVED" or verdict == "CHANGES_REQUESTED" then
      local kids = {}
      for _, t in ipairs(rt) do
        kids[#kids + 1] = thread_kid(t)
      end
      local header_str = ("@%s · %s · %s"):format(who(r), tostring(r.state or ""):lower(), reltime(r.createdAt or ""))
      local id = r.id or ("r" .. tostring(r.createdAt))
      local rx = reactions.line(r)
      if nilable(r.id) then
        subjects[id] = { id = r.id, mine = reactions.mine(r) }
        if rx then
          rx_by_id[id] = { items = { { id = r.id } } }
        end
      end
      if rx then
        header_str = header_str .. " · " .. rx
      end
      blocks[#blocks + 1] = {
        at = r.createdAt or "",
        id = id,
        header = header_str,
        body = vim.trim(r.body or "") ~= "" and split(r.body) or {},
        kids = kids,
      }
    end
  end
  table.sort(blocks, function(a, b)
    return a.at < b.at
  end)
  -- threads not tied to a shown review become their own top-level blocks.
  for _, t in ipairs(orphans) do
    local k = thread_kid(t)
    blocks[#blocks + 1] = {
      at = "",
      id = k.id,
      header = k.header,
      body = k.lines,
      body_marks = k.line_marks,
      line_subjects = k.line_subjects,
      kids = {},
    }
  end

  return {
    header = header,
    header_marks = header_marks,
    pr = pr_block,
    blocks = blocks,
    reactions = rx_by_id,
    subjects = subjects,
    jumps = jumps,
  }
end

return M
