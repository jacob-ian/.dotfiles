-- jmux: Neovim integration for the jmux worktree flow. Functionality is grouped
-- into namespaces (e.g. require("jmux").pr.view()), one per area of the flow.
local M = {}

-- pr is the lightweight GitHub PR review flow. Lazily required so the namespace
-- is cheap to touch and the pr module only loads on first use.
M.pr = require "jmux.pr"

return M
