return {
  {
    "folke/snacks.nvim",
    lazy = false,
    priority = 1000,
    ---@type snacks.Config
    opts = {
      gitbrowse = { enabled = true },
      picker = { enabled = true },
      notifier = { enabled = true },
      image = { enabled = true },
      input = { enabled = true },
    },
    dependencies = {
      "nvim-tree/nvim-web-devicons",
    },
    keys = {
      -- Picker
      {
        "<leader>ff",
        function()
          Snacks.picker.files {
            hidden = true,
          }
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
          Snacks.picker.grep {
            hidden = true,
          }
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
      {
        "<leader>gb",
        function()
          Snacks.gitbrowse {
            open = function(url)
              vim.fn.setreg("+", url)
            end,
            notify = true,
            what = "permalink",
          }
        end,
        desc = "Copy Git URL",
        mode = { "n", "v" },
      },
    },
  },
}
