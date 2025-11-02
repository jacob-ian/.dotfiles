return {
  {
    "saghen/blink.cmp",
    version = "1.*",
    dependencies = {
      { "L3MON4D3/LuaSnip", build = "make install_jsregexp" },
    },
    ---@module 'blink.cmp'
    ---@type blink.cmp.Config
    opts = {},
    opts_extend = { "sources.default" },
  },
}
