return {
  {
    src = "saghen/blink.cmp",
    version = vim.version.range "1.*",
    config = function()
      require("blink.cmp").setup {}
    end,
  },
}
