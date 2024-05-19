return {
  {
    "NeogitOrg/neogit",
    lazy = false,
    dependencies = {
      "nvim-lua/plenary.nvim",
      "sindrets/diffview.nvim",
    },
    config = function()
      local neogit = require "neogit"
      neogit.setup {
        integrations = {
          diffview = true,
          telescope = true,
        },
      }

      local telescope = require "telescope.builtin"

      vim.keymap.set("n", "<leader>gg", neogit.open)
      vim.keymap.set("n", "<leader>gb", telescope.git_branches)
      vim.keymap.set("n", "<leader>gp", function()
        neogit.action("pull", "pull", { "--verbose" })
      end)
    end,
  },
  {
    "lewis6991/gitsigns.nvim",
    config = function()
      local gitsigns = require "gitsigns"
      gitsigns.setup {}

      vim.keymap.set("n", "<leader>gB", gitsigns.toggle_current_line_blame)
    end,
  },
}
