return {
  src = "nvim-lualine/lualine.nvim",
  config = function()
    require("lualine").setup {
      options = {
        -- one statusline for the focused window (laststatus=3): splits and
        -- plugin panes carry no per-window mode/filename bars.
        globalstatus = true,
        theme = "auto",
      },
      sections = {
        lualine_a = { "mode" },
        lualine_b = { "branch", "diff", "diagnostics" },
        lualine_c = { "filename" },
        lualine_x = {},
        lualine_y = { "filetype" },
        lualine_z = { "location" },
      },
    }
  end,
}
