return {
  {                                                                                                                                                                                    │
      "folke/lazydev.nvim",                                                                                                                                                              │
      ft = { "lua" },                                                                                                                                                                    │
      opts = {                                                                                                                                                                           │
        library = {                                                                                                                                                                      │
          { path = "${3rd}/luv/library", words = { "vim%.uv" } },                                                                                                                        │
        },                                                                                                                                                                               │
      },                                                                                                                                                                                 │
    }, 
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
        root_markers = { '.git' },
      })
      vim.lsp.config("beancount", {
        settings = { journal_file = "~/acc/journal.beancount" },
      })
      vim.lsp.enable("beancount")

      vim.lsp.enable("rust_analyzer")

      vim.lsp.enable("ts_ls")

      vim.lsp.enable("biome")

      vim.lsp.config("lua_ls", {
        settings = { Lua = { diagnostics = { globals = { "vim" } } } },
      })
      vim.lsp.enable("lua_ls")

      vim.lsp.enable("gopls")

      vim.lsp.enable("sqls")

      vim.lsp.config("html", {
        filetypes = { "html", "templ" },
      })
      vim.lsp.enable("html")

      vim.lsp.enable("cssls")

      vim.lsp.enable("terraformls")

      vim.lsp.enable("dockerls")

      vim.lsp.enable("graphql")

      vim.lsp.enable("prismals")

      vim.lsp.enable("buf_ls")

      vim.lsp.config("htmx", {
        filetypes = { "html", "templ" },
      })
      vim.lsp.enable("htmx")

      vim.lsp.enable("templ")

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
      vim.lsp.enable("tailwindcss")

      vim.lsp.config("jsonls", {
        settings = {
          json = {
            schemas = require("schemastore").json.schemas(),
            validate = { enable = true },
          },
        },
      })
      vim.lsp.enable("jsonls")

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
      vim.lsp.enable("yamlls")

      -- FORMATTING
      local conform = require "conform"
      conform.setup {
        formatters_by_ft = {
          lua = { "stylua" },
          html = { "prettierd", "prettier", stop_after_first = true },
          yaml = { "prettierd", "prettier", stop_after_first = true },
          markdown = { "prettierd", "prettier", stop_after_first = true },
          templ = { "templ" },
          sql = { "sleek" },
        },
        format_on_save = {
          timeout_ms = 500,
          lsp_format = "fallback",
          stop_after_first = true,
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
