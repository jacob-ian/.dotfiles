-- Install packer.nvim
local fn = vim.fn
local install_path = fn.stdpath("data") .. "/site/pack/packer/start/packer.nvim"
if fn.empty(fn.glob(install_path)) > 0 then
	packer_bootstrap = fn.system({
		"git",
		"clone",
		"--depth",
		"1",
		"https://github.com/wbthomason/packer.nvim",
		install_path,
	})
end

-- Install and Configure Plugins
return require("packer").startup(function(use)
	use("wbthomason/packer.nvim")

	-- PDE
	use({
		"folke/tokyonight.nvim",
		config = "vim.cmd[[colorscheme tokyonight]]",
	})

	use({
		"kyazdani42/nvim-tree.lua",
		requires = {
			"kyazdani42/nvim-web-devicons",
		},
		config = function()
			require("nvim-tree").setup({
				renderer = {
					icons = {
						webdev_colors = true,
					},
				},
				filters = {
					dotfiles = false,
					custom = { ".git", "node_modules" },
					exclude = { ".gitignore", ".github" },
				},
				git = {
					ignore = false,
				},
				actions = {
					open_file = {
						quit_on_open = true,
					},
				},
			})
		end,
	})

	use({
		"nvim-lualine/lualine.nvim",
		config = function()
			require("lualine").setup({
				options = {
					theme = "tokyonight",
				},
			})
		end,
	})

	use({
		"nvim-telescope/telescope.nvim",
		requires = { "nvim-lua/plenary.nvim" },
		config = function()
			require("telescope").setup({
				defaults = { file_ignore_patterns = { "node_modules", ".git/" } },
			})
		end,
	})

	-- Language/Coding Helpers
	use({
		"hrsh7th/nvim-cmp",
		requires = {
			"hrsh7th/cmp-nvim-lsp",
			"hrsh7th/cmp-buffer",
			"hrsh7th/cmp-path",
			"hrsh7th/cmp-cmdline",
			"hrsh7th/cmp-vsnip",
			"hrsh7th/vim-vsnip",
		},
		config = function()
			local cmp = require("cmp")
			cmp.setup({
				snippet = {
					expand = function(args)
						vim.fn["vsnip#anonymous"](args.body)
					end,
				},
				mapping = cmp.mapping.preset.insert({
					["<C-b>"] = cmp.mapping.scroll_docs(-4),
					["<C-f>"] = cmp.mapping.scroll_docs(4),
					["<C-Space>"] = cmp.mapping.complete(),
					["<C-e>"] = cmp.mapping.abort(),
					["<CR>"] = cmp.mapping.confirm({ select = true }),
				}),
				sources = cmp.config.sources({
					{ name = "nvim_lsp" },
					{ name = "vsnip" },
				}, {
					{ name = "buffer" },
				}),
			})
		end,
	})

	use({
		"neovim/nvim-lspconfig",
		config = function()
			local lspconfig = require("lspconfig")
			local capabilities = require("cmp_nvim_lsp").default_capabilities()
			lspconfig.sumneko_lua.setup({
				capabilities = capabilities,
				on_attach = function(client)
					client.server_capabilities.documentFormattingProvider = false -- Use null-ls stylua
				end,
			})
			lspconfig.tsserver.setup({
				capabilities = capabilities,
				on_attach = function(client)
					client.server_capabilities.documentFormattingProvider = false -- Use null-ls prettierd
				end,
			})
			lspconfig.cssls.setup({
				capabilities = capabilities,
				on_attach = function(client)
					client.server_capabilities.documentFormattingProvider = false -- Use null-ls prettierd
				end,
			})
			lspconfig.cssmodules_ls.setup({
				capabilities = capabilities,
				on_attach = function(client)
					client.server_capabilities.documentFormattingProvider = false -- Use null-ls prettierd
				end,
			})
			lspconfig.tailwindcss.setup({
				settings = {
					tailwindCSS = {
						classAttributes = { "class", "className", "classList", "ngClass" },
						lint = {
							cssConflict = "warning",
							invalidApply = "error",
							invalidConfigPath = "error",
							invalidScreen = "error",
							invalidTailwindDirective = "error",
							invalidVariant = "error",
							recommendedVariantOrder = "warning",
						},
						validate = true,
					},
				},
			})
			lspconfig.html.setup({
				capabilities = capabilities,
				on_attach = function(client)
					client.server_capabilities.documentFormattingProvider = false -- Use null-ls prettierd
				end,
			})
			vim.cmd([[
                augroup jsonc
                    autocmd!
                    autocmd BufRead,BufNewFile *.json set filetype=jsonc
                augroup END
            ]])
			lspconfig.jsonls.setup({
				capabilities = capabilities,
				on_attach = function(client)
					client.server_capabilities.documentFormattingProvider = false -- Use null-ls prettierd
				end,
			})
			lspconfig.gopls.setup({
				capabilities = capabilities,
				cmd = { "gopls", "serve" },
				filetypes = { "go", "gomod" },
				root_dir = require("lspconfig/util").root_pattern("go.work", "go.mod", ".git"),
			})
		end,
	})

	use({
		"jose-elias-alvarez/null-ls.nvim",
		requires = { "nvim-lua/plenary.nvim" },
		config = function()
			local null_ls = require("null-ls")
			null_ls.setup({
				sources = {
					null_ls.builtins.formatting.prettierd,
					null_ls.builtins.formatting.stylua,
					null_ls.builtins.code_actions.eslint_d,
					null_ls.builtins.diagnostics.eslint_d,
					null_ls.builtins.completion.spell.with({
						filetypes = { "markdown" },
					}),
				},
			})
			vim.cmd([[
                augroup formatting
                    autocmd! 
                    autocmd BufWritePre * lua vim.lsp.buf.formatting_sync(nil, 1000)
                augroup END
            ]])
		end,
	})

	use({
		"nvim-treesitter/nvim-treesitter",
		run = ":TSUpdate",
		config = function()
			require("nvim-treesitter.configs").setup({
				ensure_installed = {
					"typescript",
					"javascript",
					"jsonc",
					"markdown",
					"tsx",
					"yaml",
					"bash",
					"comment",
					"css",
					"dockerfile",
					"go",
					"graphql",
					"html",
					"jsdoc",
					"lua",
					"php",
					"python",
					"regex",
					"scss",
				},
				highlight = {
					enable = true,
				},
				indent = {
					enable = true,
				},
			})
			local ft_to_parser = require("nvim-treesitter.parsers").filetype_to_parsername
			ft_to_parser.json = "jsonc"
		end,
	})

	use({
		"windwp/nvim-autopairs",
		config = function()
			require("nvim-autopairs").setup({
				check_ts = true,
			})
		end,
	})

	use("tpope/vim-commentary")

	-- install without yarn or npm
	use({
		"iamcco/markdown-preview.nvim",
		run = function()
			vim.fn["mkdp#util#install"]()
		end,
	})

	-- Git Helpers
	use({
		"lewis6991/gitsigns.nvim",
		config = function()
			require("gitsigns").setup({
				current_line_blame = true,
				current_line_blame_opts = {
					virt_text_pos = "right_align",
					delay = 500,
				},
			})
		end,
	})
	use("sindrets/diffview.nvim")
	use("kdheepak/lazygit.nvim")

	if packer_bootstrap then
		require("packer").sync()
	end
end)
