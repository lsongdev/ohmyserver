# Vendored browser dependencies

Served as-is so the module works offline and needs no build step.

| File            | Package               | Version | Source |
|-----------------|-----------------------|---------|--------|
| preact.mjs      | preact                | 11.0.2  | https://cdn.jsdelivr.net/npm/preact@11.0.2/dist/preact.mjs |
| hooks.mjs       | preact/hooks          | 11.0.2  | https://cdn.jsdelivr.net/npm/preact@11.0.2/hooks/dist/hooks.mjs |
| htm.mjs         | htm                   | 3.1.1   | https://cdn.jsdelivr.net/npm/htm@3.1.1/dist/htm.module.js |
| xterm.mjs       | @xterm/xterm          | 6.0.0   | https://cdn.jsdelivr.net/npm/@xterm/xterm@6.0.0/lib/xterm.mjs |
| xterm.css       | @xterm/xterm          | 6.0.0   | https://cdn.jsdelivr.net/npm/@xterm/xterm@6.0.0/css/xterm.css |
| addon-fit.mjs   | @xterm/addon-fit      | 0.11.0  | https://cdn.jsdelivr.net/npm/@xterm/addon-fit@0.11.0/lib/addon-fit.mjs |

Local modification: `hooks.mjs` imports `./preact.mjs` instead of the bare
specifier `preact`, so no import map is needed. xterm.js and its fit addon
load only when a terminal opens.
