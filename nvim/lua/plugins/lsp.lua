return {
  {
    "neovim/nvim-lspconfig",
    dependencies = {
      "folke/neodev.nvim",
      "b0o/SchemaStore.nvim",
      "stevearc/conform.nvim",
    },
    config = function()
      local capabilities = nil
      if pcall(require, "cmp_nvim_lsp") then
        capabilities = require("cmp_nvim_lsp").default_capabilities()
      end

      require("neodev").setup {}

      local lspconfig = require "lspconfig"
      vim.filetype.add { extension = { templ = "templ" } }
      vim.filetype.add { extension = { beancount = "beancount" } }
      lspconfig.beancount.setup {
        capabilities = capabilities,
        init_options = { journal_file = "~/acc/journal.beancount" },
      }
      lspconfig.rust_analyzer.setup { capabilities = capabilities }
      lspconfig.terraformls.setup { capabilities = capabilities }
      lspconfig.ts_ls.setup { capabilities = capabilities }
      lspconfig.cssls.setup { capabilities = capabilities }
      lspconfig.dockerls.setup { capabilities = capabilities }
      lspconfig.tailwindcss.setup {
        capabilities = capabilities,
        filetypes = { "templ", "typescriptreact", "react", "html" },
        settings = {
          tailwindCSS = {
            includeLanguages = {
              templ = "html",
            },
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
      }
      lspconfig.htmx.setup { capabilities = capabilities, filetypes = { "html", "templ" } }
      lspconfig.html.setup { capabilities = capabilities, filetypes = { "html", "templ" } }
      lspconfig.templ.setup { capabilities = capabilities }
      lspconfig.gopls.setup {
        capabilities = capabilities,
        cmd = { "gopls", "serve" },
        filetypes = { "go", "gomod" },
        root_dir = require("lspconfig/util").root_pattern("go.work", "go.mod", ".git"),
      }
      lspconfig.lua_ls.setup {
        capabilities = capabilities,
        settings = { Lua = { diagnostics = { globals = { "vim" } } } },
      }
      lspconfig.jsonls.setup {
        capabilities = capabilities,
        settings = {
          json = {
            schemas = require("schemastore").json.schemas(),
            validate = { enable = true },
          },
        },
      }
      lspconfig.yamlls.setup {
        capabilities = capabilities,
        settings = {
          yaml = {
            schemaStore = {
              enable = false,
              url = "",
            },
            schemas = require("schemastore").yaml.schemas(),
          },
        },
      }
      lspconfig.sqls.setup { capabilities = capabilities }

      local conform = require "conform"
      conform.setup {
        formatters_by_ft = {
          lua = { "stylua" },
          javascript = { "prettierd", "prettier", stop_after_first = true },
          typescript = { "prettierd", "prettier", stop_after_first = true },
          json = { "prettierd", "prettier", stop_after_first = true },
          html = { "prettierd", "prettier", stop_after_first = true },
          yaml = { "prettierd", "prettier", stop_after_first = true },
          markdown = { { "prettierd", "prettier" } },
          sql = { "sql_formatter" },
          templ = { "templ" },
        },
      }

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

      vim.api.nvim_create_autocmd("BufWritePre", {
        pattern = "*",
        callback = function(args)
          conform.format {
            bufnr = args.buf,
            lsp_fallback = true,
            quiet = true,
          }
        end,
      })
    end,
  },
}
