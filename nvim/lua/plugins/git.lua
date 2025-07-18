return {
  {
    "NeogitOrg/neogit",
    lazy = false,
    dependencies = {
      "nvim-lua/plenary.nvim",
      "sindrets/diffview.nvim",
      "folke/snacks.nvim", -- picker
    },
    config = function()
      local neogit = require "neogit"
      neogit.setup {
        integrations = {
          diffview = true,
          snacks = true,
        },
        console_timeout = 10000,
        graph_style = "kitty",
      }

      vim.keymap.set("n", "<leader>gg", neogit.open)
      vim.keymap.set("n", "<leader>gp", function()
        neogit.open { "pull" }
      end)
    end,
  },
  {
    "lewis6991/gitsigns.nvim",
    config = function()
      local gitsigns = require "gitsigns"
      gitsigns.setup {}
    end,
  },
}
