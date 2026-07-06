return {
  {
    src = "iamcco/markdown-preview.nvim",
    build = "cd app && yarn install",
    config = function()
      -- Read by the plugin/ file, which sources after init.lua finishes.
      vim.g.mkdp_filetypes = { "markdown" }
    end,
  },
}
