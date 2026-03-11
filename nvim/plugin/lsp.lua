vim.lsp.config("*", {
  capabilities = require("blink.cmp").get_lsp_capabilities(),
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
  "templ",
  "tailwindcss",
  "jsonls",
  "yamlls",
}

vim.api.nvim_create_autocmd("LspAttach", {
  callback = function()
    vim.keymap.set("n", "gd", vim.lsp.buf.definition, { buffer = 0 })
    vim.keymap.set("n", "gD", vim.lsp.buf.declaration, { buffer = 0 })
  end,
})
