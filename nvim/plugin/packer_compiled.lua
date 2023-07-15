-- Automatically generated packer.nvim plugin loader code

if vim.api.nvim_call_function('has', {'nvim-0.5'}) ~= 1 then
  vim.api.nvim_command('echohl WarningMsg | echom "Invalid Neovim version for packer.nvim! | echohl None"')
  return
end

vim.api.nvim_command('packadd packer.nvim')

local no_errors, error_msg = pcall(function()

_G._packer = _G._packer or {}
_G._packer.inside_compile = true

local time
local profile_info
local should_profile = false
if should_profile then
  local hrtime = vim.loop.hrtime
  profile_info = {}
  time = function(chunk, start)
    if start then
      profile_info[chunk] = hrtime()
    else
      profile_info[chunk] = (hrtime() - profile_info[chunk]) / 1e6
    end
  end
else
  time = function(chunk, start) end
end

local function save_profiles(threshold)
  local sorted_times = {}
  for chunk_name, time_taken in pairs(profile_info) do
    sorted_times[#sorted_times + 1] = {chunk_name, time_taken}
  end
  table.sort(sorted_times, function(a, b) return a[2] > b[2] end)
  local results = {}
  for i, elem in ipairs(sorted_times) do
    if not threshold or threshold and elem[2] > threshold then
      results[i] = elem[1] .. ' took ' .. elem[2] .. 'ms'
    end
  end
  if threshold then
    table.insert(results, '(Only showing plugins that took longer than ' .. threshold .. ' ms ' .. 'to load)')
  end

  _G._packer.profile_output = results
end

time([[Luarocks path setup]], true)
local package_path_str = "/Users/jacob-ian/.cache/nvim/packer_hererocks/2.1.0-beta3/share/lua/5.1/?.lua;/Users/jacob-ian/.cache/nvim/packer_hererocks/2.1.0-beta3/share/lua/5.1/?/init.lua;/Users/jacob-ian/.cache/nvim/packer_hererocks/2.1.0-beta3/lib/luarocks/rocks-5.1/?.lua;/Users/jacob-ian/.cache/nvim/packer_hererocks/2.1.0-beta3/lib/luarocks/rocks-5.1/?/init.lua"
local install_cpath_pattern = "/Users/jacob-ian/.cache/nvim/packer_hererocks/2.1.0-beta3/lib/lua/5.1/?.so"
if not string.find(package.path, package_path_str, 1, true) then
  package.path = package.path .. ';' .. package_path_str
end

if not string.find(package.cpath, install_cpath_pattern, 1, true) then
  package.cpath = package.cpath .. ';' .. install_cpath_pattern
end

time([[Luarocks path setup]], false)
time([[try_loadstring definition]], true)
local function try_loadstring(s, component, name)
  local success, result = pcall(loadstring(s), name, _G.packer_plugins[name])
  if not success then
    vim.schedule(function()
      vim.api.nvim_notify('packer.nvim: Error running ' .. component .. ' for ' .. name .. ': ' .. result, vim.log.levels.ERROR, {})
    end)
  end
  return result
end

time([[try_loadstring definition]], false)
time([[Defining packer_plugins]], true)
_G.packer_plugins = {
  ["cmp-buffer"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/cmp-buffer",
    url = "https://github.com/hrsh7th/cmp-buffer"
  },
  ["cmp-cmdline"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/cmp-cmdline",
    url = "https://github.com/hrsh7th/cmp-cmdline"
  },
  ["cmp-nvim-lsp"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/cmp-nvim-lsp",
    url = "https://github.com/hrsh7th/cmp-nvim-lsp"
  },
  ["cmp-path"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/cmp-path",
    url = "https://github.com/hrsh7th/cmp-path"
  },
  ["cmp-vsnip"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/cmp-vsnip",
    url = "https://github.com/hrsh7th/cmp-vsnip"
  },
  ["diffview.nvim"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/diffview.nvim",
    url = "https://github.com/sindrets/diffview.nvim"
  },
  ["gitsigns.nvim"] = {
    config = { "\27LJ\2\n—\1\0\0\4\0\6\0\t6\0\0\0'\2\1\0B\0\2\0029\0\2\0005\2\3\0005\3\4\0=\3\5\2B\0\2\1K\0\1\0\28current_line_blame_opts\1\0\2\18virt_text_pos\16right_align\ndelay\3ô\3\1\0\1\23current_line_blame\1\nsetup\rgitsigns\frequire\0" },
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/gitsigns.nvim",
    url = "https://github.com/lewis6991/gitsigns.nvim"
  },
  ["lazygit.nvim"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/lazygit.nvim",
    url = "https://github.com/kdheepak/lazygit.nvim"
  },
  ["lualine.nvim"] = {
    config = { "\27LJ\2\n`\0\0\4\0\6\0\t6\0\0\0'\2\1\0B\0\2\0029\0\2\0005\2\4\0005\3\3\0=\3\5\2B\0\2\1K\0\1\0\foptions\1\0\0\1\0\1\ntheme\15tokyonight\nsetup\flualine\frequire\0" },
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/lualine.nvim",
    url = "https://github.com/nvim-lualine/lualine.nvim"
  },
  ["markdown-preview.nvim"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/markdown-preview.nvim",
    url = "https://github.com/iamcco/markdown-preview.nvim"
  },
  ["neo-tree.nvim"] = {
    config = { "\27LJ\2\nÔ\3\0\0\6\0\19\0\0236\0\0\0009\0\1\0'\2\2\0B\0\2\0016\0\3\0'\2\4\0B\0\2\0029\0\5\0005\2\6\0005\3\b\0005\4\a\0=\4\t\0035\4\n\0=\4\v\3=\3\f\0025\3\16\0005\4\r\0005\5\14\0=\5\15\4=\4\17\3=\3\18\2B\0\2\1K\0\1\0\15filesystem\19filtered_items\1\0\2\26hijack_netrw_behavior\17open_default\24follow_current_file\2\17hide_by_name\1\2\0\0\17node_modules\1\0\2\20hide_gitignored\1\18hide_dotfiles\1\30default_component_configs\tname\1\0\1\19trailing_slash\2\ticon\1\0\0\1\0\4\16folder_open\bâŒ„\fdefault\bâˆ—\18folder_closed\bâ€º\17folder_empty\6-\1\0\1\22enable_git_status\2\nsetup\rneo-tree\frequire0 let g:neo_tree_remove_legacy_commands = 1 \bcmd\bvim\0" },
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/neo-tree.nvim",
    url = "https://github.com/nvim-neo-tree/neo-tree.nvim"
  },
  ["nui.nvim"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/nui.nvim",
    url = "https://github.com/MunifTanjim/nui.nvim"
  },
  ["null-ls.nvim"] = {
    config = { "\27LJ\2\nÔ\1\0\0\t\0\14\1\0226\0\0\0'\2\1\0B\0\2\0029\1\2\0005\3\f\0004\4\3\0009\5\3\0009\5\4\0059\5\5\5>\5\1\0049\5\3\0009\5\6\0059\5\a\0059\5\b\0055\a\n\0005\b\t\0=\b\v\aB\5\2\0?\5\0\0=\4\r\3B\1\2\1K\0\1\0\fsources\1\0\0\14filetypes\1\0\0\1\2\0\0\rmarkdown\twith\nspell\15completion\14prettierd\15formatting\rbuiltins\nsetup\fnull-ls\frequire\5€€À™\4\0" },
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/null-ls.nvim",
    url = "https://github.com/jose-elias-alvarez/null-ls.nvim"
  },
  ["nvim-autopairs"] = {
    config = { "\27LJ\2\nM\0\0\3\0\4\0\a6\0\0\0'\2\1\0B\0\2\0029\0\2\0005\2\3\0B\0\2\1K\0\1\0\1\0\1\rcheck_ts\2\nsetup\19nvim-autopairs\frequire\0" },
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/nvim-autopairs",
    url = "https://github.com/windwp/nvim-autopairs"
  },
  ["nvim-cmp"] = {
    config = { "\27LJ\2\n;\0\1\4\0\4\0\0066\1\0\0009\1\1\0019\1\2\0019\3\3\0B\1\2\1K\0\1\0\tbody\20vsnip#anonymous\afn\bvim¤\3\1\0\n\0\27\00046\0\0\0'\2\1\0B\0\2\0029\1\2\0005\3\6\0005\4\4\0003\5\3\0=\5\5\4=\4\a\0039\4\b\0009\4\t\0049\4\n\0045\6\f\0009\a\b\0009\a\v\a)\tüÿB\a\2\2=\a\r\0069\a\b\0009\a\v\a)\t\4\0B\a\2\2=\a\14\0069\a\b\0009\a\15\aB\a\1\2=\a\16\0069\a\b\0009\a\17\aB\a\1\2=\a\18\0069\a\b\0009\a\19\a5\t\20\0B\a\2\2=\a\21\6B\4\2\2=\4\b\0039\4\22\0009\4\23\0044\6\3\0005\a\24\0>\a\1\0065\a\25\0>\a\2\0064\a\3\0005\b\26\0>\b\1\aB\4\3\2=\4\23\3B\1\2\1K\0\1\0\1\0\1\tname\vbuffer\1\0\1\tname\nvsnip\1\0\1\tname\rnvim_lsp\fsources\vconfig\t<CR>\1\0\1\vselect\2\fconfirm\n<C-e>\nabort\14<C-Space>\rcomplete\n<C-f>\n<C-b>\1\0\0\16scroll_docs\vinsert\vpreset\fmapping\fsnippet\1\0\0\vexpand\1\0\0\0\nsetup\bcmp\frequire\0" },
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/nvim-cmp",
    url = "https://github.com/hrsh7th/nvim-cmp"
  },
  ["nvim-lspconfig"] = {
    config = { "\27LJ\2\nF\0\1\3\0\2\0\0049\1\0\0+\2\1\0=\2\1\1K\0\1\0\31documentFormattingProvider\24server_capabilitiesF\0\1\3\0\2\0\0049\1\0\0+\2\1\0=\2\1\1K\0\1\0\31documentFormattingProvider\24server_capabilitiesF\0\1\3\0\2\0\0049\1\0\0+\2\1\0=\2\1\1K\0\1\0\31documentFormattingProvider\24server_capabilitiesF\0\1\3\0\2\0\0049\1\0\0+\2\1\0=\2\1\1K\0\1\0\31documentFormattingProvider\24server_capabilitiesF\0\1\3\0\2\0\0049\1\0\0+\2\1\0=\2\1\1K\0\1\0\31documentFormattingProvider\24server_capabilitiesÕ\t\1\0\n\0B\0y6\0\0\0'\2\1\0B\0\2\0026\1\0\0'\3\2\0B\1\2\0029\1\3\1B\1\1\0029\2\4\0009\2\5\0025\4\6\0=\1\a\0045\5\b\0=\5\t\4B\2\2\0019\2\n\0009\2\5\0025\4\v\0=\1\a\4B\2\2\0019\2\f\0009\2\5\0025\4\r\0=\1\a\4B\2\2\0019\2\14\0009\2\5\0025\4\15\0=\1\a\4B\2\2\0019\2\16\0009\2\5\0025\4\17\0=\1\a\4B\2\2\0019\2\18\0009\2\5\0025\4\19\0=\1\a\4B\2\2\0019\2\20\0009\2\5\0025\4\21\0=\1\a\0043\5\22\0=\5\23\4B\2\2\0019\2\24\0009\2\5\0025\4\25\0=\1\a\4B\2\2\0019\2\26\0009\2\5\0025\4\27\0=\1\a\0043\5\28\0=\5\23\4B\2\2\0019\2\29\0009\2\5\0025\4\30\0=\1\a\0043\5\31\0=\5\23\4B\2\2\0019\2 \0009\2\5\0025\4(\0005\5&\0005\6\"\0005\a!\0=\a#\0065\a$\0=\a%\6=\6'\5=\5\t\4B\2\2\0019\2)\0009\2\5\0025\4*\0=\1\a\0043\5+\0=\5\23\4B\2\2\0019\2,\0009\2\5\0025\4-\0=\1\a\0043\5.\0=\5\23\0045\0054\0005\0062\0004\a\3\0005\b0\0005\t/\0=\t1\b>\b\1\a=\a3\6=\0065\5=\5\t\4B\2\2\0019\0026\0009\2\5\0025\0047\0=\1\a\0045\0058\0=\0059\0045\5:\0=\5;\0046\5\0\0'\a<\0B\5\2\0029\5=\5'\a>\0'\b?\0'\t@\0B\5\4\2=\5A\4B\2\2\1K\0\1\0\rroot_dir\t.git\vgo.mod\fgo.work\17root_pattern\19lspconfig/util\14filetypes\1\3\0\0\ago\ngomod\bcmd\1\3\0\0\ngopls\nserve\1\0\0\ngopls\tjson\1\0\0\fschemas\1\0\0\14fileMatch\1\0\1\burl.https://json.schemastore.org/package.json\1\2\0\0\17package.json\0\1\0\0\vjsonls\0\1\0\0\thtml\1\0\0\16tailwindCSS\1\0\0\tlint\1\0\a\16cssConflict\fwarning\28recommendedVariantOrder\fwarning\19invalidVariant\nerror\29invalidTailwindDirective\nerror\18invalidScreen\nerror\22invalidConfigPath\nerror\17invalidApply\nerror\20classAttributes\1\0\1\rvalidate\2\1\5\0\0\nclass\14className\14classList\fngClass\16tailwindcss\0\1\0\0\18cssmodules_ls\0\1\0\0\ncssls\1\0\0\rphpactor\14on_attach\0\1\0\0\rtsserver\1\0\0\vlua_ls\1\0\0\rdockerls\1\0\0\vmetals\1\0\0\16terraformls\1\0\0\18rust_analyzer\rsettings\1\0\2\rnodePath\5\19packageManager\tyarn\17capabilities\1\0\0\nsetup\veslint\25default_capabilities\17cmp_nvim_lsp\14lspconfig\frequire\0" },
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/nvim-lspconfig",
    url = "https://github.com/neovim/nvim-lspconfig"
  },
  ["nvim-treesitter"] = {
    config = { "\27LJ\2\n€\3\0\0\4\0\14\0\0196\0\0\0'\2\1\0B\0\2\0029\0\2\0005\2\4\0005\3\3\0=\3\5\0025\3\6\0=\3\a\0025\3\b\0=\3\t\2B\0\2\0016\0\0\0'\2\n\0B\0\2\0029\0\v\0'\1\r\0=\1\f\0K\0\1\0\njsonc\tjson\27filetype_to_parsername\28nvim-treesitter.parsers\vindent\1\0\1\venable\2\14highlight\1\0\1\venable\2\21ensure_installed\1\0\0\1\23\0\0\15typescript\15javascript\njsonc\rmarkdown\btsx\tyaml\tbash\fcomment\bcss\15dockerfile\ago\fgraphql\thtml\njsdoc\blua\bphp\vpython\nregex\tscss\nscala\bhcl\14terraform\nsetup\28nvim-treesitter.configs\frequire\0" },
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/nvim-treesitter",
    url = "https://github.com/nvim-treesitter/nvim-treesitter"
  },
  ["packer.nvim"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/packer.nvim",
    url = "https://github.com/wbthomason/packer.nvim"
  },
  ["plenary.nvim"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/plenary.nvim",
    url = "https://github.com/nvim-lua/plenary.nvim"
  },
  ["telescope.nvim"] = {
    config = { "\27LJ\2\n†\1\0\0\5\0\b\0\v6\0\0\0'\2\1\0B\0\2\0029\0\2\0005\2\6\0005\3\4\0005\4\3\0=\4\5\3=\3\a\2B\0\2\1K\0\1\0\rdefaults\1\0\0\25file_ignore_patterns\1\0\0\1\3\0\0\17node_modules\n.git/\nsetup\14telescope\frequire\0" },
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/telescope.nvim",
    url = "https://github.com/nvim-telescope/telescope.nvim"
  },
  ["tmux.nvim"] = {
    config = { "\27LJ\2\n2\0\0\3\0\3\0\0066\0\0\0'\2\1\0B\0\2\0029\0\2\0B\0\1\1K\0\1\0\nsetup\ttmux\frequire\0" },
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/tmux.nvim",
    url = "https://github.com/aserowy/tmux.nvim"
  },
  ["tokyonight.nvim"] = {
    config = { "vim.cmd[[colorscheme tokyonight-night]]" },
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/tokyonight.nvim",
    url = "https://github.com/folke/tokyonight.nvim"
  },
  ["vim-commentary"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/vim-commentary",
    url = "https://github.com/tpope/vim-commentary"
  },
  ["vim-vsnip"] = {
    loaded = true,
    path = "/Users/jacob-ian/.local/share/nvim/site/pack/packer/start/vim-vsnip",
    url = "https://github.com/hrsh7th/vim-vsnip"
  }
}

time([[Defining packer_plugins]], false)
-- Config for: nvim-treesitter
time([[Config for nvim-treesitter]], true)
try_loadstring("\27LJ\2\n€\3\0\0\4\0\14\0\0196\0\0\0'\2\1\0B\0\2\0029\0\2\0005\2\4\0005\3\3\0=\3\5\0025\3\6\0=\3\a\0025\3\b\0=\3\t\2B\0\2\0016\0\0\0'\2\n\0B\0\2\0029\0\v\0'\1\r\0=\1\f\0K\0\1\0\njsonc\tjson\27filetype_to_parsername\28nvim-treesitter.parsers\vindent\1\0\1\venable\2\14highlight\1\0\1\venable\2\21ensure_installed\1\0\0\1\23\0\0\15typescript\15javascript\njsonc\rmarkdown\btsx\tyaml\tbash\fcomment\bcss\15dockerfile\ago\fgraphql\thtml\njsdoc\blua\bphp\vpython\nregex\tscss\nscala\bhcl\14terraform\nsetup\28nvim-treesitter.configs\frequire\0", "config", "nvim-treesitter")
time([[Config for nvim-treesitter]], false)
-- Config for: null-ls.nvim
time([[Config for null-ls.nvim]], true)
try_loadstring("\27LJ\2\nÔ\1\0\0\t\0\14\1\0226\0\0\0'\2\1\0B\0\2\0029\1\2\0005\3\f\0004\4\3\0009\5\3\0009\5\4\0059\5\5\5>\5\1\0049\5\3\0009\5\6\0059\5\a\0059\5\b\0055\a\n\0005\b\t\0=\b\v\aB\5\2\0?\5\0\0=\4\r\3B\1\2\1K\0\1\0\fsources\1\0\0\14filetypes\1\0\0\1\2\0\0\rmarkdown\twith\nspell\15completion\14prettierd\15formatting\rbuiltins\nsetup\fnull-ls\frequire\5€€À™\4\0", "config", "null-ls.nvim")
time([[Config for null-ls.nvim]], false)
-- Config for: lualine.nvim
time([[Config for lualine.nvim]], true)
try_loadstring("\27LJ\2\n`\0\0\4\0\6\0\t6\0\0\0'\2\1\0B\0\2\0029\0\2\0005\2\4\0005\3\3\0=\3\5\2B\0\2\1K\0\1\0\foptions\1\0\0\1\0\1\ntheme\15tokyonight\nsetup\flualine\frequire\0", "config", "lualine.nvim")
time([[Config for lualine.nvim]], false)
-- Config for: telescope.nvim
time([[Config for telescope.nvim]], true)
try_loadstring("\27LJ\2\n†\1\0\0\5\0\b\0\v6\0\0\0'\2\1\0B\0\2\0029\0\2\0005\2\6\0005\3\4\0005\4\3\0=\4\5\3=\3\a\2B\0\2\1K\0\1\0\rdefaults\1\0\0\25file_ignore_patterns\1\0\0\1\3\0\0\17node_modules\n.git/\nsetup\14telescope\frequire\0", "config", "telescope.nvim")
time([[Config for telescope.nvim]], false)
-- Config for: nvim-cmp
time([[Config for nvim-cmp]], true)
try_loadstring("\27LJ\2\n;\0\1\4\0\4\0\0066\1\0\0009\1\1\0019\1\2\0019\3\3\0B\1\2\1K\0\1\0\tbody\20vsnip#anonymous\afn\bvim¤\3\1\0\n\0\27\00046\0\0\0'\2\1\0B\0\2\0029\1\2\0005\3\6\0005\4\4\0003\5\3\0=\5\5\4=\4\a\0039\4\b\0009\4\t\0049\4\n\0045\6\f\0009\a\b\0009\a\v\a)\tüÿB\a\2\2=\a\r\0069\a\b\0009\a\v\a)\t\4\0B\a\2\2=\a\14\0069\a\b\0009\a\15\aB\a\1\2=\a\16\0069\a\b\0009\a\17\aB\a\1\2=\a\18\0069\a\b\0009\a\19\a5\t\20\0B\a\2\2=\a\21\6B\4\2\2=\4\b\0039\4\22\0009\4\23\0044\6\3\0005\a\24\0>\a\1\0065\a\25\0>\a\2\0064\a\3\0005\b\26\0>\b\1\aB\4\3\2=\4\23\3B\1\2\1K\0\1\0\1\0\1\tname\vbuffer\1\0\1\tname\nvsnip\1\0\1\tname\rnvim_lsp\fsources\vconfig\t<CR>\1\0\1\vselect\2\fconfirm\n<C-e>\nabort\14<C-Space>\rcomplete\n<C-f>\n<C-b>\1\0\0\16scroll_docs\vinsert\vpreset\fmapping\fsnippet\1\0\0\vexpand\1\0\0\0\nsetup\bcmp\frequire\0", "config", "nvim-cmp")
time([[Config for nvim-cmp]], false)
-- Config for: tmux.nvim
time([[Config for tmux.nvim]], true)
try_loadstring("\27LJ\2\n2\0\0\3\0\3\0\0066\0\0\0'\2\1\0B\0\2\0029\0\2\0B\0\1\1K\0\1\0\nsetup\ttmux\frequire\0", "config", "tmux.nvim")
time([[Config for tmux.nvim]], false)
-- Config for: neo-tree.nvim
time([[Config for neo-tree.nvim]], true)
try_loadstring("\27LJ\2\nÔ\3\0\0\6\0\19\0\0236\0\0\0009\0\1\0'\2\2\0B\0\2\0016\0\3\0'\2\4\0B\0\2\0029\0\5\0005\2\6\0005\3\b\0005\4\a\0=\4\t\0035\4\n\0=\4\v\3=\3\f\0025\3\16\0005\4\r\0005\5\14\0=\5\15\4=\4\17\3=\3\18\2B\0\2\1K\0\1\0\15filesystem\19filtered_items\1\0\2\26hijack_netrw_behavior\17open_default\24follow_current_file\2\17hide_by_name\1\2\0\0\17node_modules\1\0\2\20hide_gitignored\1\18hide_dotfiles\1\30default_component_configs\tname\1\0\1\19trailing_slash\2\ticon\1\0\0\1\0\4\16folder_open\bâŒ„\fdefault\bâˆ—\18folder_closed\bâ€º\17folder_empty\6-\1\0\1\22enable_git_status\2\nsetup\rneo-tree\frequire0 let g:neo_tree_remove_legacy_commands = 1 \bcmd\bvim\0", "config", "neo-tree.nvim")
time([[Config for neo-tree.nvim]], false)
-- Config for: nvim-lspconfig
time([[Config for nvim-lspconfig]], true)
try_loadstring("\27LJ\2\nF\0\1\3\0\2\0\0049\1\0\0+\2\1\0=\2\1\1K\0\1\0\31documentFormattingProvider\24server_capabilitiesF\0\1\3\0\2\0\0049\1\0\0+\2\1\0=\2\1\1K\0\1\0\31documentFormattingProvider\24server_capabilitiesF\0\1\3\0\2\0\0049\1\0\0+\2\1\0=\2\1\1K\0\1\0\31documentFormattingProvider\24server_capabilitiesF\0\1\3\0\2\0\0049\1\0\0+\2\1\0=\2\1\1K\0\1\0\31documentFormattingProvider\24server_capabilitiesF\0\1\3\0\2\0\0049\1\0\0+\2\1\0=\2\1\1K\0\1\0\31documentFormattingProvider\24server_capabilitiesÕ\t\1\0\n\0B\0y6\0\0\0'\2\1\0B\0\2\0026\1\0\0'\3\2\0B\1\2\0029\1\3\1B\1\1\0029\2\4\0009\2\5\0025\4\6\0=\1\a\0045\5\b\0=\5\t\4B\2\2\0019\2\n\0009\2\5\0025\4\v\0=\1\a\4B\2\2\0019\2\f\0009\2\5\0025\4\r\0=\1\a\4B\2\2\0019\2\14\0009\2\5\0025\4\15\0=\1\a\4B\2\2\0019\2\16\0009\2\5\0025\4\17\0=\1\a\4B\2\2\0019\2\18\0009\2\5\0025\4\19\0=\1\a\4B\2\2\0019\2\20\0009\2\5\0025\4\21\0=\1\a\0043\5\22\0=\5\23\4B\2\2\0019\2\24\0009\2\5\0025\4\25\0=\1\a\4B\2\2\0019\2\26\0009\2\5\0025\4\27\0=\1\a\0043\5\28\0=\5\23\4B\2\2\0019\2\29\0009\2\5\0025\4\30\0=\1\a\0043\5\31\0=\5\23\4B\2\2\0019\2 \0009\2\5\0025\4(\0005\5&\0005\6\"\0005\a!\0=\a#\0065\a$\0=\a%\6=\6'\5=\5\t\4B\2\2\0019\2)\0009\2\5\0025\4*\0=\1\a\0043\5+\0=\5\23\4B\2\2\0019\2,\0009\2\5\0025\4-\0=\1\a\0043\5.\0=\5\23\0045\0054\0005\0062\0004\a\3\0005\b0\0005\t/\0=\t1\b>\b\1\a=\a3\6=\0065\5=\5\t\4B\2\2\0019\0026\0009\2\5\0025\0047\0=\1\a\0045\0058\0=\0059\0045\5:\0=\5;\0046\5\0\0'\a<\0B\5\2\0029\5=\5'\a>\0'\b?\0'\t@\0B\5\4\2=\5A\4B\2\2\1K\0\1\0\rroot_dir\t.git\vgo.mod\fgo.work\17root_pattern\19lspconfig/util\14filetypes\1\3\0\0\ago\ngomod\bcmd\1\3\0\0\ngopls\nserve\1\0\0\ngopls\tjson\1\0\0\fschemas\1\0\0\14fileMatch\1\0\1\burl.https://json.schemastore.org/package.json\1\2\0\0\17package.json\0\1\0\0\vjsonls\0\1\0\0\thtml\1\0\0\16tailwindCSS\1\0\0\tlint\1\0\a\16cssConflict\fwarning\28recommendedVariantOrder\fwarning\19invalidVariant\nerror\29invalidTailwindDirective\nerror\18invalidScreen\nerror\22invalidConfigPath\nerror\17invalidApply\nerror\20classAttributes\1\0\1\rvalidate\2\1\5\0\0\nclass\14className\14classList\fngClass\16tailwindcss\0\1\0\0\18cssmodules_ls\0\1\0\0\ncssls\1\0\0\rphpactor\14on_attach\0\1\0\0\rtsserver\1\0\0\vlua_ls\1\0\0\rdockerls\1\0\0\vmetals\1\0\0\16terraformls\1\0\0\18rust_analyzer\rsettings\1\0\2\rnodePath\5\19packageManager\tyarn\17capabilities\1\0\0\nsetup\veslint\25default_capabilities\17cmp_nvim_lsp\14lspconfig\frequire\0", "config", "nvim-lspconfig")
time([[Config for nvim-lspconfig]], false)
-- Config for: gitsigns.nvim
time([[Config for gitsigns.nvim]], true)
try_loadstring("\27LJ\2\n—\1\0\0\4\0\6\0\t6\0\0\0'\2\1\0B\0\2\0029\0\2\0005\2\3\0005\3\4\0=\3\5\2B\0\2\1K\0\1\0\28current_line_blame_opts\1\0\2\18virt_text_pos\16right_align\ndelay\3ô\3\1\0\1\23current_line_blame\1\nsetup\rgitsigns\frequire\0", "config", "gitsigns.nvim")
time([[Config for gitsigns.nvim]], false)
-- Config for: tokyonight.nvim
time([[Config for tokyonight.nvim]], true)
vim.cmd[[colorscheme tokyonight-night]]
time([[Config for tokyonight.nvim]], false)
-- Config for: nvim-autopairs
time([[Config for nvim-autopairs]], true)
try_loadstring("\27LJ\2\nM\0\0\3\0\4\0\a6\0\0\0'\2\1\0B\0\2\0029\0\2\0005\2\3\0B\0\2\1K\0\1\0\1\0\1\rcheck_ts\2\nsetup\19nvim-autopairs\frequire\0", "config", "nvim-autopairs")
time([[Config for nvim-autopairs]], false)

_G._packer.inside_compile = false
if _G._packer.needs_bufread == true then
  vim.cmd("doautocmd BufRead")
end
_G._packer.needs_bufread = false

if should_profile then save_profiles() end

end)

if not no_errors then
  error_msg = error_msg:gsub('"', '\\"')
  vim.api.nvim_command('echohl ErrorMsg | echom "Error in packer_compiled: '..error_msg..'" | echom "Please check your config for correctness" | echohl None')
end
