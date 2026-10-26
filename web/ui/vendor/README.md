# Vendored browser dependencies

Served as-is so the module works offline and needs no build step.

| File       | Package      | Version | Source |
|------------|--------------|---------|--------|
| preact.mjs | preact       | 11.0.2  | https://cdn.jsdelivr.net/npm/preact@11.0.2/dist/preact.mjs |
| hooks.mjs  | preact/hooks | 11.0.2  | https://cdn.jsdelivr.net/npm/preact@11.0.2/hooks/dist/hooks.mjs |
| htm.mjs    | htm          | 3.1.1   | https://cdn.jsdelivr.net/npm/htm@3.1.1/dist/htm.module.js |
| stylesheets.css | @lsongdev/stylesheets | 1.0.0 | https://cdn.jsdelivr.net/npm/@lsongdev/stylesheets@1.0.0/ |

Local modification: `hooks.mjs` imports `./preact.mjs` instead of the bare
specifier `preact`, so no import map is needed.

`stylesheets.css` concatenates base.css (without its palette import),
card.css, input.css, button.css, table.css, badge.css, alert.css, dialog.css,
theme/light.css and theme/dark.css from the package, with their `@import`
lines removed; each part is marked with a comment.
