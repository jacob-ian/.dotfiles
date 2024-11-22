local opt = vim.opt

opt.termguicolors = true
opt.hidden = true
opt.swapfile = false

opt.tabstop = 4
opt.softtabstop = 4
opt.shiftwidth = 4
opt.expandtab = true

opt.number = true
opt.scrolloff = 16
opt.wrap = false
opt.cursorline = true
opt.incsearch = true
opt.hlsearch = false
opt.ignorecase = true
opt.smartcase = true

opt.colorcolumn = "90"
opt.signcolumn = "yes"

-- Make sql stuff better
vim.g.omni_sql_no_default_maps = 1
