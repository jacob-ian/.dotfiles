return {
  {
    "coder/claudecode.nvim",
    dependencies = { "folke/snacks.nvim" },
    opts = {
      auto_start = true,
      terminal = {
        provider = "none",
      },
    },
    config = true,
    keys = {
      { "<leader>a", nil, desc = "AI/Claude Code" },
      { "<leader>as", "<cmd>ClaudeCodeSend<cr>", mode = "v", desc = "Send selection to Claude" },
      { "<leader>ab", "<cmd>ClaudeCodeAdd %<cr>", desc = "Add current buffer to Claude" },
      { "<leader>aa", "<cmd>ClaudeCodeDiffAccept<cr>", desc = "Accept Claude diff" },
      { "<leader>ad", "<cmd>ClaudeCodeDiffDeny<cr>", desc = "Deny Claude diff" },
    },
  },
}
