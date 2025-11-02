return {
  {
    "neovim/nvim-lspconfig",
    dependencies = {
      "b0o/SchemaStore.nvim",
      "stevearc/conform.nvim",
    },
    config = function()
      -- EXTRA FILETYPES
      vim.filetype.add { extension = { templ = "templ" } }
      vim.filetype.add { extension = { beancount = "beancount" } }

      -- LSP CONFIG
      vim.lsp.config("*", {
        root_markers = { ".git" },
      })
      vim.lsp.config("beancount", {
        settings = { journal_file = "~/acc/journal.beancount" },
      })
      vim.lsp.enable "beancount"

      vim.lsp.enable "rust_analyzer"

      vim.lsp.enable "ts_ls"

      vim.lsp.enable "biome"

      vim.lsp.config("lua_ls", {
        settings = { Lua = { diagnostics = { globals = { "vim" } } } },
      })
      vim.lsp.enable "lua_ls"

      vim.lsp.config("gopls", {
        settings = {
          gopls = {
            staticcheck = true,
            analyses = {
              ST1003 = true,
            },
          },
        },
      })
      vim.lsp.enable "gopls"

      vim.lsp.enable "sqls"

      vim.lsp.config("html", {
        filetypes = { "html", "templ" },
      })
      vim.lsp.enable "html"

      vim.lsp.enable "cssls"

      vim.lsp.enable "terraformls"

      vim.lsp.enable "dockerls"

      vim.lsp.enable "graphql"

      vim.lsp.enable "prismals"

      vim.lsp.enable "buf_ls"

      vim.lsp.config("htmx", {
        filetypes = { "html", "templ" },
      })
      vim.lsp.enable "htmx"

      vim.lsp.enable "templ"

      vim.lsp.config("tailwindcss", {
        filetypes = { "templ", "typescriptreact", "react", "html" },
        settings = {
          tailwindCSS = {
            includeLanguages = {
              templ = "html",
            },
            classFunctions = { "tw", "clsx" },
            classAttributes = { "class", "className", "classList", "ngClass" },
            lint = {
              cssConflict = "warning",
              invalidApply = "error",
              invalidConfigPath = "error",
              invalidScreen = "error",
              invalidTailwindDirective = "error",
              invalidVariant = "error",
              recommendedVariantOrder = "warning",
            },
            validate = true,
          },
        },
      })
      vim.lsp.enable "tailwindcss"

      vim.lsp.config("jsonls", {
        settings = {
          json = {
            schemas = require("schemastore").json.schemas(),
            validate = { enable = true },
          },
        },
      })
      vim.lsp.enable "jsonls"

      vim.lsp.config("yamlls", {
        settings = {
          yaml = {
            schemaStore = {
              enable = false,
              url = "",
            },
            schemas = require("schemastore").yaml.schemas(),
          },
        },
      })
      vim.lsp.enable "yamlls"

      -- FORMATTING
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
        },
        format_on_save = {
          timeout_ms = 500,
          lsp_format = "fallback",
        },
      }

      -- LSP KEYMAPS
      vim.api.nvim_create_autocmd("LspAttach", {
        callback = function()
          vim.opt_local.omnifunc = "v:lua.vim.lsp.omnifunc"
          vim.keymap.set("n", "gd", vim.lsp.buf.definition, { buffer = 0 })
          vim.keymap.set("n", "gD", vim.lsp.buf.declaration, { buffer = 0 })
          vim.keymap.set("n", "K", vim.lsp.buf.hover, { buffer = 0 })
          vim.keymap.set("n", "<leader>gr", vim.lsp.buf.references, { buffer = 0 })
          vim.keymap.set("n", "<leader>la", vim.lsp.buf.code_action, { buffer = 0 })
          vim.keymap.set("n", "<leader>lr", vim.lsp.buf.rename, { buffer = 0 })
          vim.keymap.set("n", "<leader>do", vim.diagnostic.open_float, { buffer = 0 })
          vim.keymap.set("n", "<leader>d[", vim.diagnostic.goto_prev, { buffer = 0 })
          vim.keymap.set("n", "<leader>d]", vim.diagnostic.goto_next, { buffer = 0 })
        end,
      })
    end,
  },
}
