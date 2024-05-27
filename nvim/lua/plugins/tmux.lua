return {
  {
    "aserowy/tmux.nvim",
    lazy = false,
    config = function()
      require("tmux").setup()
    end,
  },
  {
    "tpope/vim-obsession",
    lazy = false,
  },
}
