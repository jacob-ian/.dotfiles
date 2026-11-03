local opt = vim.opt

opt.swapfile = false

opt.tabstop = 2
opt.softtabstop = 2
opt.shiftwidth = 2
opt.expandtab = true

opt.number = true
opt.signcolumn = "yes"
opt.scrolloff = 16
opt.wrap = false
opt.cursorline = true
opt.hlsearch = false
opt.ignorecase = true
opt.smartcase = true

opt.colorcolumn = "90"

-- Reload files changed externally (e.g. by Claude Code)
vim.api.nvim_create_autocmd({ "FocusGained", "BufEnter" }, {
  command = "checktime",
})

-- Make sql stuff better
vim.g.omni_sql_no_default_maps = 1

-- Closing a tab returns to the last-used one, so q in a jmux, neogit or
-- diffview tab lands back where it was opened from, not on the next tab over.
opt.tabclose = "uselast"
