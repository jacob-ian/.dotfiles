return {
  {
    "nvim-treesitter/nvim-treesitter",
    lazy = false,
    build = ":TSUpdate",
    config = function()
      local configs = require "nvim-treesitter.configs"
      configs.setup {
        ensure_installed = {
          "typescript",
          "tsx",
          "javascript",
          "json",
          "jsonc",
          "yaml",
          "markdown",
          "bash",
          "css",
          "scss",
          "dockerfile",
          "go",
          "rust",
          "html",
          "jsdoc",
          "lua",
          "php",
          "python",
          "regex",
          "terraform",
          "templ",
          "beancount",
        },
        highlight = {
          enable = true,
        },
        indent = {
          enable = true,
        },
      }
    end,
  },
}
