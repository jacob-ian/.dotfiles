-- LSP config
vim.lsp.config("*", {
  root_markers = { ".git" },
})

vim.lsp.enable {
  "beancount",
  "rust_analyzer",
  "ts_ls",
  "biome",
  "lua_ls",
  "gopls",
  "sqls",
  "html",
  "cssls",
  "terraformls",
  "dockerls",
  "graphql",
  "prismals",
  "buf_ls",
  "htmx",
  "templ",
  "tailwindcss",
  "jsonls",
  "yamlls",
}

-- LSP keymaps
vim.api.nvim_create_autocmd("LspAttach", {
  callback = function()
    vim.opt_local.omnifunc = "v:lua.vim.lsp.omnifunc"
    vim.keymap.set("n", "<leader>la", vim.lsp.buf.code_action, { buffer = 0 })
    vim.keymap.set("n", "<leader>lr", vim.lsp.buf.rename, { buffer = 0 })
    vim.keymap.set("n", "<leader>do", vim.diagnostic.open_float, { buffer = 0 })
    vim.keymap.set("n", "<leader>d[", function()
      vim.diagnostic.jump { count = -1 }
    end, { buffer = 0 })
    vim.keymap.set("n", "<leader>d]", function()
      vim.diagnostic.jump { count = 1 }
    end, { buffer = 0 })
  end,
})
