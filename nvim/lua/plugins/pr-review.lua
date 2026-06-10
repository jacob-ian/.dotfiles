-- Local plugin: the lightweight PR review flow (code in ../../pr-review/). Lazy
-- loaded by its keymaps; diffview (a neogit dependency) is already up, so
-- <leader>pc can read its diff buffers.
return {
  dir = vim.fn.stdpath "config" .. "/pr-review",
  keys = {
    {
      "<leader>pc",
      function()
        require("pr_review").add_comment()
      end,
      mode = { "n", "x" },
      desc = "PR: stage review comment",
    },
    {
      "<leader>ps",
      function()
        require("pr_review").submit()
      end,
      desc = "PR: submit review",
    },
    {
      "<leader>pl",
      function()
        require("pr_review").list()
      end,
      desc = "PR: list pending comments",
    },
    {
      "<leader>px",
      function()
        require("pr_review").discard()
      end,
      desc = "PR: discard pending comments",
    },
    {
      "<leader>pv",
      function()
        require("pr_review").view()
      end,
      desc = "PR: view in popup",
    },
    {
      "<leader>po",
      function()
        require("pr_review").browser()
      end,
      desc = "PR: open in browser",
    },
  },
}
