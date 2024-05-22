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
        hidden = true,
      },
    }

    local builtin = require "telescope.builtin"
    vim.keymap.set("n", "<leader>ff", function()
      builtin.find_files({ hidden = true })
    end)
    vim.keymap.set("n", "<leader>FF", builtin.live_grep)
    vim.keymap.set("n", "<leader>fh", builtin.help_tags)
  end,
}
