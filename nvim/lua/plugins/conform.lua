return {
  {
    "stevearc/conform.nvim",
    lazy = false,
    config = function()
      local conform = require "conform"

      local formatWithBiomeOrPrettier = function(bufnr)
        if conform.get_formatter_info("prettier", bufnr).available then
          return { "prettier" }
        else
          return { "biome", "biome-organize-imports" }
        end
      end

      conform.setup {
        formatters = {
          prettier = {
            require_cwd = true,
          },
        },
        formatters_by_ft = {
          lua = { "stylua" },
          markdown = { "prettierd", "prettier", stop_after_first = true },
          yaml = { "prettierd", "prettier", stop_after_first = true },
          typescript = formatWithBiomeOrPrettier,
          typescriptreact = formatWithBiomeOrPrettier,
          json = formatWithBiomeOrPrettier,
          graphql = formatWithBiomeOrPrettier,
          html = formatWithBiomeOrPrettier,
          sql = { "pg_format" },
        },
        format_on_save = {
          timeout_ms = 500,
          lsp_format = "fallback",
        },
      }
    end,
  },
}
