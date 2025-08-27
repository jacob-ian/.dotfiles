return {
  {
    "catppuccin/nvim",
    name = "catppuccin",
    lazy = false,
    priority = 1000,
    config = function()
      require("catppuccin").setup {
        auto_integrations = true,
        color_overrides = {
          mocha = {
            base = "#000000",
            mantle = "#000000",
            crust = "#000000",
          },
        },
      }
      vim.cmd.colorscheme "catppuccin-mocha"
    end,
  },
  "EdenEast/nightfox.nvim",
  "sainnhe/gruvbox-material",
  "ellisonleao/gruvbox.nvim",
  "navarasu/onedark.nvim",
  "AlexvZyl/nordic.nvim",
  "neanias/everforest-nvim",
  "projekt0n/github-nvim-theme",
  "rebelot/kanagawa.nvim",
}
