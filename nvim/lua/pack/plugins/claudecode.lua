return {
  {
    src = "coder/claudecode.nvim",
    config = function()
      require("claudecode").setup {
        auto_start = true,
        terminal = {
          provider = "none",
        },
      }

      vim.keymap.set("v", "<leader>as", "<cmd>ClaudeCodeSend<cr>", { desc = "Send selection to Claude" })
      vim.keymap.set("n", "<leader>ab", "<cmd>ClaudeCodeAdd %<cr>", { desc = "Add current buffer to Claude" })
      vim.keymap.set("n", "<leader>aa", "<cmd>ClaudeCodeDiffAccept<cr>", { desc = "Accept Claude diff" })
      vim.keymap.set("n", "<leader>ad", "<cmd>ClaudeCodeDiffDeny<cr>", { desc = "Deny Claude diff" })
    end,
  },
}
