# fzt-browser

Browser renderer for fzt. Compiles the Go engine to WASM and bundles the JavaScript/CSS shim that talks to it. Consumed by `my-homepage` and `fzt-showcase`.

## Contents

- `main.go` — Go-side WASM entry point (`//go:build js && wasm`). Imports `fzt/core` and `fzt-terminal/tui` for headless session rendering. Exposes a `fzt` global on window.
- `fzt-terminal.js` — Core browser terminal: ANSI parser, grid renderer (styled HTML spans), font metrics, keyboard forwarding, resize observer. Factory: `createFztTerminal(container, options)`.
- `fzt-web.js` — Higher-level wrapper with Catppuccin Mocha defaults (palette, Perfect DOS VGA 437 font, Nerd Font icons). Factory: `createFztWeb(container, options)`.
- `fzt-dom-renderer.js` — Alternative DOM renderer using structured data API (`getVisibleRows`, `getPromptState`, `getUIState`) instead of ANSI parsing. Renders native DOM elements with CSS classes.
- `fzt-terminal.css` — CRT terminal styles: scanline overlay, vignette, cursor blink, custom properties (`--fzt-*`).

## WASM API

The Go code exposes a global `fzt` object:

- `fzt.loadYAML(yaml)` — parse YAML items
- `fzt.setFrontend({name, version})` — register frontend identity
- `fzt.addCommands([{name, description, action}])` — register frontend commands for `:` palette
- `fzt.setLabel(text)` — set the top-left border label
- `fzt.setStatus(msg, style)` — write to the title bar status area. `style` maps to `core.State.TitleStyle` (0=default cyan, 1=green success, 2=red error, 3=neutral slate). Requires an active session; returns a frame for immediate render.
- `fzt.clearStatus()` — remove the title override, restoring the default title.
- `fzt.init(cols, rows)` — create session, returns `{ansi, cursorX, cursorY}`
- `fzt.handleKey(key, ctrl, shift)` — process keyboard event, returns frame + action. The `ctrl` bool is accepted but ignored — the ecosystem retired Ctrl bindings entirely (my-homepage#24); any Ctrl+letter is a silent no-op. Kept in the signature so JS callers don't break; can be dropped whenever downstream consumers update.
- `fzt.clickRow(row)` — process mouse click on visual row
- `fzt.resize(cols, rows)` — resize terminal
- `fzt.getVisibleRows()` / `fzt.getPromptState()` / `fzt.getUIState()` — structured data API for the DOM renderer

`EnvTags: ["wasm", "browser"]` is set on the session config so browser-inappropriate commands (e.g. the terminal-only `update`) are filtered out of the palette.

## Dependencies

- `github.com/romaine-life/fzt` — engine (state, scoring, tree logic, YAML parsing, render abstractions)
- `github.com/romaine-life/fzt-terminal` — headless session + palette injection via tui (which in turn pulls in fzt-frontend)
- `github.com/gdamore/tcell/v2` — terminal screen library (used by the headless session)

## CI

Three workflows, all manually triggered (Claude-driven):

- **`.github/workflows/dispatch.yml`** (`workflow_dispatch`) — dependency bump path. Calls `go-dependency-update-template`: bumps `github.com/romaine-life/fzt` + `github.com/romaine-life/fzt-terminal` in go.mod, commits, pushes, then delegates to `release-and-dispatch-template` to cut the release.
- **`.github/workflows/release.yml`** (`workflow_dispatch`) — direct-push release path for code changes. Calls `release-and-dispatch-template` directly. Shared `release-tag` concurrency group with `dispatch.yml`.
- **`.github/workflows/build.yml`** (fires on `workflow_dispatch` from the release template with the new tag as input) — compiles `fzt.wasm` via `GOOS=js GOARCH=wasm` (injecting `render.Version` and `frontend.EngineVersion` via ldflags), packages the four JS/CSS files, and `gh release upload`s them to the already-created release. Tag is passed by value so no sha-drift race.

Downstream consumers (`fzt-showcase`, `my-homepage`) download the release assets at deploy time and are updated manually — no `repository_dispatch` fan-out. Template lives at `nelsong6/pipeline-templates`.

## History

Extracted from `romaine-life/fzt-terminal` on 2026-04-16 as part of the fzt repo split. Was `fzt-terminal/web/` + `fzt-terminal/cmd/wasm/`. The grammar goal: each fzt repo names exactly one thing.
