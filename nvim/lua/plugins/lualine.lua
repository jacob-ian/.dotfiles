return {
  "nvim-lualine/lualine.nvim",
  config = function()
    require("lualine").setup {
      options = {
        globalstatus = false,
        theme = "auto",
      },
    }
  end,
}
