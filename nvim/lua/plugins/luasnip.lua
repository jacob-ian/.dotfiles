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
    loader.load { paths = { "~/.config/nvim/snippets" } }
  end,
}
