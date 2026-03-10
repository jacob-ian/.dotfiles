return {
  filetypes = { "templ", "typescriptreact", "react", "html" },
  settings = {
    tailwindCSS = {
      includeLanguages = {
        templ = "html",
      },
      classFunctions = { "tw", "clsx" },
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
}
