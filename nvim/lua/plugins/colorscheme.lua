return {
  {
    "rebelot/kanagawa.nvim",
    lazy = false,
    config = function()
      vim.api.nvim_create_augroup("nobg", { clear = true })
      vim.api.nvim_create_autocmd({ "ColorScheme" }, {
        desc = "Make all backgrounds transparent",
        group = "nobg",
        pattern = "*",
        callback = function()
          vim.api.nvim_set_hl(0, "Normal", { bg = "NONE", ctermbg = "NONE" })
          vim.api.nvim_set_hl(0, "NeoTreeNormal", { bg = "NONE", ctermbg = "NONE" })
          vim.api.nvim_set_hl(0, "NeoTreeNormalNC", { bg = "NONE", ctermbg = "NONE" })
        end,
      })
      vim.cmd "colorscheme kanagawa-dragon"
    end,
  },
}
