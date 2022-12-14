-- Key Mappings
vim.g.mapleader = " "

function nnoremap(shortcut, command)
	vim.api.nvim_set_keymap("n", shortcut, command, { noremap = true, silent = true })
end

-- NvimTree
nnoremap("<leader>n", ":NvimTreeToggle<cr>")

-- LSP
nnoremap("<leader>lf", ":lua vim.lsp.buf.format(nil, 1000)<cr>")
nnoremap("<leader>li", ":lua vim.lsp.buf.implementation()<cr>")
nnoremap("<leader>ld", ":lua vim.lsp.buf.definition()<cr>")
nnoremap("<leader>ln", ":lua vim.lsp.buf.rename()<cr>")
nnoremap("<leader>lr", ":lua vim.lsp.buf.references()<cr>")
nnoremap("<leader>lh", ":lua vim.lsp.buf.hover()<cr>")
nnoremap("<leader>ls", ":lua vim.lsp.buf.signature_help()<cr>")
nnoremap("<leader>la", ":lua vim.lsp.buf.code_action()<cr>")
nnoremap("<leader>do", ":lua vim.diagnostic.open_float()<cr>")
nnoremap("<leader>d[", ":lua vim.diagnostic.goto_prev()<cr>")
nnoremap("<leader>d]", ":lua vim.diagnostic.goto_next()<cr>")

-- Telescope
nnoremap("<leader>ff", ":Telescope find_files hidden=true<cr>")
nnoremap("<leader>FF", ":Telescope live_grep hidden=true<cr>")
nnoremap("<leader>fb", ":Telescope buffers<cr>")
nnoremap("<leader>fh", ":Telescope help_tags<cr>")

-- Diffview
nnoremap("<leader>dvo", ":DiffviewOpen<cr>")
nnoremap("<leader>dvc", ":DiffviewClose<cr>")
nnoremap("<leader>dvr", ":DiffviewRefresh<cr>")

nnoremap("<leader>lg", ":LazyGit<cr>")

-- Misc
nnoremap("<leader>gf", ":e <cfile><cr>")
