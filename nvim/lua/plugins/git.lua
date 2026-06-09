return {
  {
    "NeogitOrg/neogit",
    lazy = false,
    dependencies = {
      "nvim-lua/plenary.nvim",
      "sindrets/diffview.nvim",
      "folke/snacks.nvim",
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
  {
    "sindrets/diffview.nvim",
    keys = {
      {
        "<leader>gd",
        function()
          local out = vim.fn.systemlist { "git", "rev-parse", "--abbrev-ref", "origin/HEAD" }
          local base = out[1]
          if vim.v.shell_error ~= 0 or not base or base == "" then
            base = "origin/main"
          end
          vim.cmd("DiffviewOpen " .. base .. "...HEAD")
        end,
        desc = "Diffview: PR diff vs default branch",
      },
      { "<leader>gD", "<cmd>DiffviewClose<cr>", desc = "Diffview: close" },
    },
  },
}
