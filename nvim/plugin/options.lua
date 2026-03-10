local opt = vim.opt

opt.swapfile = false

opt.tabstop = 2
opt.softtabstop = 2
opt.shiftwidth = 2
opt.expandtab = true

opt.number = true
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
