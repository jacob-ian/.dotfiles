return {
  {
    "folke/snacks.nvim",
    lazy = false,
    priority = 1000,
    ---@type snacks.Config
    opts = {
      gitbrowse = { enabled = true },
      picker = { enabled = true },
    },
    keys = {
      -- Picker
      {
        "<leader>ff",
        function()
          Snacks.picker.files()
        end,
        desc = "Find Files",
      },
      {
        "<leader>fb",
        function()
          Snacks.picker.buffers()
        end,
        desc = "Buffers",
      },
      {
        "<leader>FF",
        function()
          Snacks.picker.grep()
        end,
        desc = "Grep",
      },

      -- Gitbrowse
      {
        "<leader>gB",
        function()
          Snacks.gitbrowse()
        end,
        desc = "Git Browse",
        mode = { "n", "v" },
      },
    },
  },
}
