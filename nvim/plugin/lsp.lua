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
  callback = function(args)
    vim.keymap.set("n", "gd", vim.lsp.buf.definition, { buffer = 0 })
    vim.keymap.set("n", "gD", vim.lsp.buf.declaration, { buffer = 0 })

    -- terraform-ls overflows deltaStartChar in semantic tokens for files
    -- with heredocs (terraform-ls#2122), breaking the highlighter. Drop the
    -- semantic-token layer; completion/diagnostics/hover are unaffected.
    local client = vim.lsp.get_client_by_id(args.data.client_id)
    if client and client.name:match("terraform") then
      client.server_capabilities.semanticTokensProvider = nil
    end
  end,
})
