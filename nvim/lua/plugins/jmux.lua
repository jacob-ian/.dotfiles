-- Local plugin: jmux worktree integration (code in ../../jmux/). The PR review
-- flow lives under the `pr` namespace. Lazy loaded by its keymaps; diffview (a
-- neogit dependency) is already up, so <leader>pc can read its diff buffers.
return {
  dir = vim.fn.stdpath "config" .. "/jmux",
  -- snacks powers the multiline comment input; diffview provides the diff buffers
  -- add_comment reads. Declared so they're loaded before the first keymap fires.
  dependencies = { "folke/snacks.nvim", "sindrets/diffview.nvim" },
  keys = {
    {
      "<leader>pc",
      function()
        require("jmux").pr.add_comment()
      end,
      mode = { "n", "x" },
      desc = "PR: stage review comment",
    },
    {
      "<leader>pp",
      function()
        require("jmux").pr.review()
      end,
      desc = "PR: review and submit",
    },
    {
      "<leader>pv",
      function()
        require("jmux").pr.view()
      end,
      desc = "PR: view in popup",
    },
    {
      "<leader>po",
      function()
        require("jmux").pr.browser()
      end,
      desc = "PR: open in browser",
    },
  },
}
