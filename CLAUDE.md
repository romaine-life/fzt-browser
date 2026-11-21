# fzt-browser

Browser renderer for fzt. Compiles the Go engine to WASM and bundles the JavaScript/CSS shim that talks to it. Consumed by `my-homepage` and `fzt-showcase`.

## Contents

- `main.go` — Go-side WASM entry point (`//go:build js && wasm`); `version.go` hands the release the suite stamps into `main.version` to `render.Version`. Imports `fzt/core` and `fzt-terminal/tui` for headless session rendering. Exposes a `fzt` global on window.
The JavaScript, CSS and fonts live in `release/assets/go/`, which the release suite copies into every release beside the WASM:

- `fzt-terminal.js` — Core browser terminal: ANSI parser, grid renderer (styled HTML spans), font metrics, keyboard forwarding, resize observer. Factory: `createFztTerminal(container, options)`.
- `fzt-web.js` — Higher-level wrapper with Catppuccin Mocha defaults (palette, Perfect DOS VGA 437 font, Nerd Font icons). Factory: `createFztWeb(container, options)`.
- `fzt-dom-renderer.js` — Alternative DOM renderer using structured data API (`getVisibleRows`, `getPromptState`, `getUIState`) instead of ANSI parsing. Renders native DOM elements with CSS classes.
- `fzt-terminal.css` — CRT terminal styles: scanline overlay, vignette, cursor blink, custom properties (`--fzt-*`).
- `PerfectDOSVGA437.ttf`, `SymbolsNerdFontMono-Regular.ttf` — the text and icon fonts `fzt-web.js` names; `fzt-terminal.css` declares them by these relative paths, so a page serving the release assets from one folder gets them.
- `wasm_exec.js` — Go's WASM runtime (the `Go` global), released from the toolchain that built `fzt.wasm` so a page always loads the matching one. Not checked in; the release suite copies it from `go env GOROOT` for every js/wasm build.

## WASM API

The Go code exposes a global `fzt` object:

- `fzt.loadYAML(yaml)` — parse YAML items
- `fzt.setFrontend({name, version})` — register frontend identity
- `fzt.addCommands([{name, description, action}])` — register frontend commands for `:` palette
- `fzt.setLabel(text)` — set the top-left border label
- `fzt.setStatus(msg, style)` — write to the title bar status area. `style` maps to `core.State.TitleStyle` (0=default cyan, 1=green success, 2=red error, 3=neutral slate). Requires an active session; returns a frame for immediate render.
- `fzt.clearStatus()` — remove the title override, restoring the default title.
- `fzt.init(cols, rows)` — create session, returns `{ansi, cursorX, cursorY}`
- `fzt.handleKey(key, ctrl, shift)` — process keyboard event, returns frame + action. With `ctrl`, `c` becomes Ctrl+C, which fzt treats as quit from any depth (the host gets `cancel`, as for Escape at the top; FZT2-7), and `u` becomes Ctrl+U, which erases the search line (FT-13); every other Ctrl+letter is a silent no-op, the rest of the Ctrl bindings retired by my-homepage#24.
- `fzt.clickRow(row)` — process mouse click on visual row
- `fzt.resize(cols, rows)` — resize terminal
- `fzt.getVisibleRows()` / `fzt.getPromptState()` / `fzt.getUIState()` — structured data API for the DOM renderer

`EnvTags: ["wasm", "browser"]` is set on the session config so browser-inappropriate commands (e.g. the terminal-only `update`) are filtered out of the palette.

## Dependencies

- `github.com/romaine-life/fzt` — engine (state, scoring, tree logic, YAML parsing, render abstractions)
- `github.com/romaine-life/fzt-terminal` — headless session + palette injection via tui (which in turn pulls in fzt-frontend)
- `github.com/gdamore/tcell/v2` — terminal screen library (used by the headless session)

## CI

Forgejo is the home for checks, dependency updates and releases. The repo keeps no
workflows of its own: its CI is pipeline-templates' `go` and `release` suites, set by
its entry in pipeline-templates' `fleet.json` (PIPT-12).

- **Pull requests:** gofmt, and `go vet` with `GOOS=js GOARCH=wasm` (the only
  platform this code builds for).
- **Merges:** each merge that changes the code or `release/assets/go/` releases the
  next `v0.1.N`: `fzt.wasm`, the toolchain's `wasm_exec.js` and the six files in
  `release/assets/go/`.
- **Take-ups:** `fleet.json` names `github.com/romaine-life/fzt` and
  `github.com/romaine-life/fzt-terminal` as its `go.dependencies`; strip-owner starts
  the Go suite's take-up when it delivers a change to either.

Run `ci-check` before a push. The release assets are `fzt.wasm`, `fzt-terminal.js`,
`fzt-terminal.css`, `fzt-web.js`, `fzt-dom-renderer.js`, `wasm_exec.js`,
`PerfectDOSVGA437.ttf`, and `SymbolsNerdFontMono-Regular.ttf`.

Each release also publishes the npm package `@romaine-life/fzt-browser` to Forgejo's
registry at the release's version: `package.json` lists `dist`, which holds the release
assets. Repositories that use it (landing-page, my-homepage, fzt-desktop) list it in their
own `package.json`, and each release proposes it to them as a take-up PR (pipeline-templates
`suites/README.md`, npm packages on Forgejo; PIPT-32).

## History

Extracted from `romaine-life/fzt-terminal` on 2026-04-16 as part of the fzt repo split. Was `fzt-terminal/web/` + `fzt-terminal/cmd/wasm/`. The grammar goal: each fzt repo names exactly one thing.
