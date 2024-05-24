return {
  "nvim-telescope/telescope.nvim",
  branch = "0.1.x",
  dependencies = {
    "nvim-lua/plenary.nvim",
  },
  config = function()
    require("telescope").setup {
      defaults = {
        file_ignore_patterns = { "node_modules/*" },
      },
      pickers = {
        find_files = {
          find_command = { "rg", "--files", "--hidden" }
        }
      }
    }

    local builtin = require "telescope.builtin"
    vim.keymap.set("n", "<leader>ff", builtin.find_files)
    vim.keymap.set("n", "<leader>FF", builtin.live_grep)
    vim.keymap.set("n", "<leader>fh", builtin.help_tags)
  end,
}
