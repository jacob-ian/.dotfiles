return {
  {
    src = "webhooked/kanso.nvim",
    priority = 1000,
    config = function()
      require("kanso").load "ink"
      -- kanso paints the picker's selected mark in the background colour
      -- (SnacksPickerSelected = { fg = theme.ui.bg }), so Tab-marks are
      -- invisible; give them an accent instead.
      vim.api.nvim_set_hl(0, "SnacksPickerSelected", { link = "Special" })
    end,
  },
  -- {
  --   src = "catppuccin/nvim",
  --   name = "catppuccin",
  --   priority = 1000,
  --   config = function()
  --     require("catppuccin").setup {
  --       auto_integrations = true,
  --       color_overrides = {
  --         mocha = {
  --           base = "#000000",
  --           mantle = "#000000",
  --           crust = "#000000",
  --         },
  --       },
  --     }
  --     vim.cmd.colorscheme "catppuccin-mocha"
  --   end,
  -- },
  -- "EdenEast/nightfox.nvim",
  -- "sainnhe/gruvbox-material",
  -- "ellisonleao/gruvbox.nvim",
  -- "navarasu/onedark.nvim",
  -- "AlexvZyl/nordic.nvim",
  -- "neanias/everforest-nvim",
  -- "projekt0n/github-nvim-theme",
  -- "rebelot/kanagawa.nvim",
}
