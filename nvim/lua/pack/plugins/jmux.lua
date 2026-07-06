-- Local plugin: jmux worktree integration (code in nvim/jmux/). The PR review
-- flow lives under the `pr` namespace. require("jmux") resolves lazily at
-- keypress; snacks (multiline comment input) and diffview (diff buffers
-- add_comment reads) are configured by then.
return {
  dir = vim.fn.stdpath "config" .. "/jmux",
  config = function()
    vim.keymap.set({ "n", "x" }, "<leader>pc", function()
      require("jmux").pr.add_comment()
    end, { desc = "PR: stage review comment" })
    vim.keymap.set({ "n", "x" }, "<leader>ps", function()
      require("jmux").pr.suggest()
    end, { desc = "PR: stage suggestion" })
    vim.keymap.set("n", "<leader>pd", function()
      require("jmux").pr.diff()
    end, { desc = "PR: open diff" })
    vim.keymap.set("n", "<leader>pp", function()
      require("jmux").pr.review()
    end, { desc = "PR: review and submit" })
    vim.keymap.set("n", "<leader>pv", function()
      require("jmux").pr.view()
    end, { desc = "PR: view in popup" })
    vim.keymap.set("n", "<leader>po", function()
      require("jmux").pr.browser()
    end, { desc = "PR: open in browser" })
  end,
}
