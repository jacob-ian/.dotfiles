return {
  "L3MON4D3/LuaSnip",
  build = "make install_jsregexp",
  lazy = false,
  config = function()
    local ls = require "luasnip"
    ls.setup {
      update_events = "TextChanged,TextChangedI",
    }
    local loader = require "luasnip.loaders.from_lua"
    loader.lazy_load { paths = { "~/.config/nvim/snippets", "~/snippets" } }

    vim.keymap.set({ "i", "s" }, "<c-l>", function()
      if ls.expand_or_jumpable() then
        ls.expand_or_jump()
      end
    end, { silent = true })

    vim.keymap.set({ "i", "s" }, "<c-h>", function()
      if ls.jumpable(-1) then
        ls.jump(-1)
      end
    end, { silent = true })

    vim.keymap.set({ "i", "s" }, "<c-e>", function()
      if ls.choice_active() then
        ls.change_choice(1)
      end
    end, { silent = true })
  end,
}
