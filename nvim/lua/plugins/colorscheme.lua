return {
  {
    "sainnhe/gruvbox-material",
    lazy = false,
    priority = 1000,
    config = function()
      vim.g.gruvbox_material_enable_italic = true
      vim.g.background = "dark"
      vim.g.gruvbox_material_background = "medium"
      vim.cmd "colorscheme gruvbox-material"
    end,
  },
  "ellisonleao/gruvbox.nvim",
  "navarasu/onedark.nvim",
  "AlexvZyl/nordic.nvim",
  "neanias/everforest-nvim",
  "projekt0n/github-nvim-theme",
  "rebelot/kanagawa.nvim",
}
