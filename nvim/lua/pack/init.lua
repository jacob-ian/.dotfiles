-- Plugin loader built on vim.pack (:help vim.pack).
--
-- Collects specs from every file in lua/pack/plugins/. A file returns one
-- spec or a list of specs:
--   src      "owner/repo" (expanded to github.com) or a full git URI
--   dir      local plugin directory, put on 'runtimepath' instead of vim.pack
--   name     override for the plugin's directory name
--   version  branch/tag/commit string, or vim.version.range()
--   priority higher configures first (default 0); ties keep alphabetical
--            file order, then spec order within the file
--   build    shell command, or ":Cmd" for an Ex command, run on install/update
--   config   function run after all plugins are on the runtimepath

local specs = {}
local plugin_dir = vim.fn.stdpath "config" .. "/lua/pack/plugins"
local files = vim.fn.readdir(plugin_dir)
table.sort(files)
for _, file in ipairs(files) do
  local spec = require("pack.plugins." .. file:gsub("%.lua$", ""))
  if spec.src or spec.dir then
    spec = { spec }
  end
  vim.list_extend(specs, spec)
end

for index, spec in ipairs(specs) do
  spec._index = index
end
table.sort(specs, function(a, b)
  local pa, pb = a.priority or 0, b.priority or 0
  if pa ~= pb then
    return pa > pb
  end
  return a._index < b._index
end)

-- Registered before the first vim.pack.add() so hooks also fire for installs
-- bootstrapped from the lockfile.
vim.api.nvim_create_autocmd("PackChanged", {
  group = vim.api.nvim_create_augroup("pack-build", {}),
  callback = function(ev)
    local build = (ev.data.spec.data or {}).build
    if not build or (ev.data.kind ~= "install" and ev.data.kind ~= "update") then
      return
    end
    if build:sub(1, 1) == ":" then
      if not ev.data.active then
        vim.cmd.packadd(ev.data.spec.name)
      end
      vim.cmd(build:sub(2))
    else
      local result = vim.system({ "sh", "-c", build }, { cwd = ev.data.path }):wait()
      if result.code ~= 0 then
        local msg = "pack: build failed for %s:\n%s"
        vim.notify(msg:format(ev.data.spec.name, result.stderr or ""), vim.log.levels.ERROR)
      end
    end
  end,
})

local pack_specs = {}
for _, spec in ipairs(specs) do
  if spec.dir then
    vim.opt.runtimepath:append(spec.dir)
  else
    pack_specs[#pack_specs + 1] = {
      src = spec.src:find "://" and spec.src or ("https://github.com/" .. spec.src),
      name = spec.name,
      version = spec.version,
      data = { build = spec.build },
    }
  end
end
vim.pack.add(pack_specs, { confirm = false })

for _, spec in ipairs(specs) do
  if spec.config then
    spec.config()
  end
end
