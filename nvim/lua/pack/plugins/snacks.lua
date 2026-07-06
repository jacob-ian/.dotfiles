return {
  {
    src = "echasnovski/mini.icons",
    priority = 1000,
    config = function()
      require("mini.icons").setup()
      MiniIcons.mock_nvim_web_devicons()
    end,
  },
  {
    src = "folke/snacks.nvim",
    priority = 1000,
    config = function()
      require("snacks").setup {
        gitbrowse = { enabled = true },
        picker = { enabled = true },
        notifier = { enabled = true },
        image = { enabled = true },
        input = { enabled = true },
      }

      -- Picker
      vim.keymap.set("n", "<leader>ff", function()
        Snacks.picker.files {
          hidden = true,
        }
      end, { desc = "Find Files" })
      vim.keymap.set("n", "<leader>fb", function()
        Snacks.picker.buffers()
      end, { desc = "Buffers" })
      vim.keymap.set("n", "<leader>FF", function()
        Snacks.picker.grep {
          hidden = true,
        }
      end, { desc = "Grep" })

      -- Gitbrowse
      vim.keymap.set({ "n", "v" }, "<leader>gB", function()
        Snacks.gitbrowse()
      end, { desc = "Git Browse" })
      vim.keymap.set({ "n", "v" }, "<leader>gb", function()
        Snacks.gitbrowse {
          open = function(url)
            vim.fn.setreg("+", url)
            vim.notify "Copied permalink to clipboard"
          end,
          notify = false,
          what = "permalink",
        }
      end, { desc = "Copy Git permalink" })
    end,
  },
}
