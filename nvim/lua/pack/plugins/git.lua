return {
  { src = "nvim-lua/plenary.nvim" },
  { src = "sindrets/diffview.nvim" },
  {
    src = "NeogitOrg/neogit",
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
    src = "lewis6991/gitsigns.nvim",
    config = function()
      local gitsigns = require "gitsigns"
      gitsigns.setup {}
    end,
  },
}
