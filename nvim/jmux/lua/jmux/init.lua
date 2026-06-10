-- jmux: Neovim integration for the jmux worktree flow. Functionality is grouped
-- into namespaces (e.g. require("jmux").pr.view()), one per area of the flow.
local M = {}

-- pr is the lightweight GitHub PR review flow. The whole plugin is lazy-loaded by
-- its keymaps (see lua/plugins/jmux.lua), so requiring the submodule here is fine:
-- nothing touches jmux until the first <leader>p* press.
M.pr = require "jmux.pr"

return M
