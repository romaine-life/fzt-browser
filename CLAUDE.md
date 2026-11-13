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

Forgejo is the home for checks, dependency updates and releases. Go module paths
remain `github.com/romaine-life/...`; each job reads their source from Forgejo with
a job-scoped Git URL rewrite and `PIPELINE_DISPATCH_TOKEN`.

- **`.forgejo/workflows/ci.yml`** runs on every pull request and main push. Its
  `changes` job checks CI fetches and skips the build on a pull request that
  changes only documentation; `build` vets and builds WASM with the pinned engine
  version and checks that the five release assets are present. A manual run with a
  `tag` input can resume uploading an incomplete release; existing assets must match
  the build byte for byte.
- **`.forgejo/workflows/release.yml`** is manual. It builds first, reserves the next
  `v0.1.N` tag at the checked-out commit, stages all five assets on a draft release,
  and publishes only when they are present. Re-running an already complete HEAD
  release does nothing. It runs inline because Forgejo's reusable workflow lookup
  cannot resolve the private organization reliably.
- **`.forgejo/workflows/dispatch.yml`** is the dependency take-up, through
  pipeline-templates' take-up template. strip-owner starts it when it delivers a
  change to `fzt` or `fzt-terminal`: it opens a PR taking them at their latest and
  has Forgejo merge it once CI passes, and strip-owner starts Release once it has
  (PIPT-3). It never pushes directly to main.

Run a workflow with an authenticated POST to
`https://forgejo.romaine.life/api/v1/repos/romaine-life/fzt-browser/actions/workflows/release.yml/dispatches`
with JSON body `{"ref":"main"}`.
The release assets remain `fzt.wasm`, `fzt-terminal.js`, `fzt-terminal.css`,
`fzt-web.js`, and `fzt-dom-renderer.js`.

Downstream consumers (`fzt-showcase`, `my-homepage`) download the release assets at
deploy time and are updated manually; there is no dispatch fan-out. The GitHub
workflows remain enabled during migration until Forgejo's main CI and release pass.

## History

Extracted from `romaine-life/fzt-terminal` on 2026-04-16 as part of the fzt repo split. Was `fzt-terminal/web/` + `fzt-terminal/cmd/wasm/`. The grammar goal: each fzt repo names exactly one thing.
