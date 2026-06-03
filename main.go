//go:build js && wasm

package main

import (
	"strings"
	"syscall/js"

	"github.com/gdamore/tcell/v2"
	"github.com/romaine-life/fzt-terminal/tui"
	"github.com/romaine-life/fzt/core"
	"github.com/romaine-life/fzt/render"
)

// Lifecycle: JS must call loadYAML, then optionally setFrontend/addCommands/
// hidePalette, then init to create a session. The pending* variables buffer
// frontend identity and commands because InjectCommandFolder runs during init
// and reads them from State. After init, pending values are consumed and have
// no further effect.
var (
	currentItems       []core.Item
	session            *tui.Session
	pendingCommands    []core.CommandItem
	pendingFrontend    struct{ name, version string }
	pendingHidePalette bool
)

func main() {
	js.Global().Set("fzt", js.ValueOf(map[string]interface{}{
		"init":            js.FuncOf(initSession),
		"handleKey":       js.FuncOf(handleKey),
		"clickRow":        js.FuncOf(clickRow),
		"resize":          js.FuncOf(resize),
		"loadYAML":        js.FuncOf(loadYAML),
		"setLabel":        js.FuncOf(setLabel),
		"setIdentity":     js.FuncOf(setIdentity),
		"addCommands":     js.FuncOf(addCommands),
		"setFrontend":     js.FuncOf(setFrontend),
		"hidePalette":     js.FuncOf(hidePalette),
		"setStatus":       js.FuncOf(setStatus),
		"clearStatus":     js.FuncOf(clearStatus),
		"enterPromptMode": js.FuncOf(enterPromptMode),
		"getVisibleRows":  js.FuncOf(getVisibleRows),
		"getPromptState":  js.FuncOf(getPromptState),
		"getUIState":      js.FuncOf(getUIState),
	}))
	select {}
}

// setLabel sets a label string displayed on the top-left border.
// Args: label (string)
// Returns: SessionFrame if session exists, null otherwise
func setLabel(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return jsError("setLabel requires a label string")
	}
	label := args[0].String()
	if session != nil {
		session.SetLabel(label)
		frame := session.Render()
		return frameToJS(frame)
	}
	// Store for later — will be applied when session is created
	pendingLabel = label
	return js.Null()
}

var pendingLabel string

// setIdentity sets the loaded identity string consumed by the :whoami palette
// leaf, which emits it as a one-shot title-bar status. Unlike setLabel, this
// is not rendered anywhere automatically — only when the user invokes whoami.
// Args: identity (string)
// Returns: null (no visible state change, no re-render needed)
func setIdentity(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return jsError("setIdentity requires an identity string")
	}
	identity := args[0].String()
	if session != nil {
		session.State().IdentityLabel = identity
		return js.Null()
	}
	pendingIdentity = identity
	return js.Null()
}

var pendingIdentity string

// addCommands registers frontend-specific commands for the `:` palette.
// Args: commands (array of {name: string, description: string, action: string})
// Must be called before init().
func addCommands(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return jsError("addCommands requires an array of command objects")
	}
	arr := args[0]
	length := arr.Length()
	pendingCommands = make([]core.CommandItem, 0, length)
	for i := 0; i < length; i++ {
		obj := arr.Index(i)
		pendingCommands = append(pendingCommands, core.CommandItem{
			Name:        obj.Get("name").String(),
			Description: obj.Get("description").String(),
			Action:      obj.Get("action").String(),
		})
	}
	return js.Null()
}

// setFrontend registers the frontend name and version.
// Args: {name: string, version: string}
// Must be called before init().
func setFrontend(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return jsError("setFrontend requires an object with name and version")
	}
	obj := args[0]
	pendingFrontend.name = obj.Get("name").String()
	pendingFrontend.version = obj.Get("version").String()
	return js.Null()
}

// hidePalette suppresses the visible `:` root row. The palette and its
// commands still exist in the tree and remain reachable by typing `:`;
// only the root row is hidden. Must be called before init().
// No args, no return value.
func hidePalette(this js.Value, args []js.Value) interface{} {
	pendingHidePalette = true
	return js.Null()
}

// setStatus writes a message to the title bar status area. style maps to
// core.State.TitleStyle: 0=default cyan, 1=green success, 2=red error,
// 3=neutral slate. Requires an active session — no pending buffering.
// Returns a SessionFrame (ansi, cursorX, cursorY) for the caller to
// render; returns null if no session is active yet.
func setStatus(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return jsError("setStatus requires a message string")
	}
	msg := args[0].String()
	style := 0
	if len(args) >= 2 {
		style = args[1].Int()
	}
	if session == nil {
		return js.Null()
	}
	session.State().SetTitle(msg, style)
	return frameToJS(session.Render())
}

// clearStatus removes the title override, letting the default title render
// again. Returns a SessionFrame for the caller to render; null if no
// session is active.
func clearStatus(this js.Value, args []js.Value) interface{} {
	if session == nil {
		return js.Null()
	}
	session.State().ClearTitle()
	return frameToJS(session.Render())
}

func enterPromptMode(this js.Value, args []js.Value) interface{} {
	if session == nil {
		return js.Null()
	}
	if len(args) < 2 {
		return jsError("enterPromptMode requires (mode, action[, icon, placeholder])")
	}
	mode := args[0].String()
	action := args[1].String()
	var icon rune
	if len(args) >= 3 && args[2].Type() == js.TypeString {
		runes := []rune(args[2].String())
		if len(runes) > 0 {
			icon = runes[0]
		}
	}
	placeholder := ""
	if len(args) >= 4 && args[3].Type() == js.TypeString {
		placeholder = args[3].String()
	}
	session.State().EnterPromptMode(mode, action, icon, placeholder)
	return frameToJS(session.Render())
}

// loadYAML parses YAML and stores items, but does not create a session.
func loadYAML(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return jsError("loadYAML requires a YAML string argument")
	}
	items, err := core.LoadYAMLFromString(args[0].String())
	if err != nil {
		return jsError(err.Error())
	}
	currentItems = items
	return js.Null()
}

// initSession creates a new headless TUI session in tree view mode.
// Args: cols (int), rows (int)
// Returns: {ansi: string, cursorX: int, cursorY: int}
func initSession(this js.Value, args []js.Value) interface{} {
	if len(args) < 2 {
		return jsError("init requires (cols, rows)")
	}
	cols := args[0].Int()
	rows := args[1].Int()

	if len(currentItems) == 0 {
		return jsError("no items loaded — call loadYAML first")
	}

	headerItem := core.Item{Fields: []string{"Name", "Description"}, Depth: -1}
	items := append([]core.Item{headerItem}, currentItems...)

	cfg := tui.Config{
		Layout:       "reverse",
		Border:       true,
		Tiered:       true,
		DepthPenalty: 5,
		HeaderLines:  1,
		EnvTags:      []string{"wasm", "browser"},
	}

	// Apply frontend identity before session creation so InjectCommandFolder picks it up
	if pendingFrontend.name != "" {
		cfg.FrontendName = pendingFrontend.name
		cfg.FrontendVersion = pendingFrontend.version
	}
	if len(pendingCommands) > 0 {
		cfg.FrontendCommands = pendingCommands
	}
	if pendingHidePalette {
		cfg.HidePalette = true
	}

	session = tui.NewTreeSession(items, cfg, cols, rows)
	if pendingLabel != "" {
		session.SetLabel(pendingLabel)
		pendingLabel = ""
	}
	if pendingIdentity != "" {
		session.State().IdentityLabel = pendingIdentity
		pendingIdentity = ""
	}
	frame := session.Render()
	return frameToJS(frame)
}

// clickRow handles a mouse click on a visual row in tree mode.
// Args: row (int, 0-based visual row)
// Returns: {ansi: string, cursorX: int, cursorY: int, action: string, url: string}
func clickRow(this js.Value, args []js.Value) interface{} {
	if session == nil {
		return jsError("session not initialized")
	}
	if len(args) < 1 {
		return jsError("clickRow requires (row)")
	}
	row := args[0].Int()

	frame, result := session.ClickRowResult(row)
	action := result.Action

	obj := js.Global().Get("Object").New()
	obj.Set("ansi", frame.ANSI)
	obj.Set("cursorX", frame.CursorX)
	obj.Set("cursorY", frame.CursorY)
	obj.Set("action", action)
	if strings.HasPrefix(action, "select:") {
		obj.Set("url", render.ActionURL(result))
	}
	return obj
}

// handleKey processes a keyboard event.
// Args: key (string, e.g. "ArrowUp", "Enter", "a"), ctrl (bool), shift (bool)
// Returns: {ansi: string, cursorX: int, cursorY: int, action: string}
func handleKey(this js.Value, args []js.Value) interface{} {
	if session == nil {
		return jsError("session not initialized")
	}
	if len(args) < 3 {
		return jsError("handleKey requires (key, ctrl, shift)")
	}

	keyStr := args[0].String()
	ctrl := args[1].Bool()
	shift := args[2].Bool()

	key, ch := translateKey(keyStr, ctrl, shift)
	if key == tcell.KeyRune && ch == 0 {
		// Unrecognized key — ignore
		return js.Null()
	}

	frame, result := session.HandleKeyResult(key, ch, shift)
	action := result.Action

	// Route "select:" actions inside the `:` palette through ProcessAction so
	// internally-handled commands (:whoami, ::version, ::validate, etc.)
	// actually execute instead of leaking as raw "select:<name>" strings to
	// JavaScript. ProcessAction may mutate state (SetTitle, clear search),
	// so re-render afterwards to capture those mutations in the returned
	// frame. In the tcell event loop this happens via a natural redraw tick;
	// the WASM bridge has no tick, so the re-render has to be explicit.
	action = tui.ProcessAction(session.State(), action)
	if action == "" {
		frame = session.Render()
	}

	obj := js.Global().Get("Object").New()
	obj.Set("ansi", frame.ANSI)
	obj.Set("cursorX", frame.CursorX)
	obj.Set("cursorY", frame.CursorY)
	obj.Set("action", action)
	if strings.HasPrefix(action, "select:") {
		obj.Set("url", render.ActionURL(result))
	}
	return obj
}

// resize changes the terminal dimensions.
// Args: cols (int), rows (int)
// Returns: {ansi: string, cursorX: int, cursorY: int}
func resize(this js.Value, args []js.Value) interface{} {
	if session == nil {
		return jsError("session not initialized")
	}
	if len(args) < 2 {
		return jsError("resize requires (cols, rows)")
	}
	cols := args[0].Int()
	rows := args[1].Int()

	frame := session.Resize(cols, rows)
	return frameToJS(frame)
}

// translateKey maps browser key event properties to tcell key + rune.
func translateKey(key string, ctrl, shift bool) (tcell.Key, rune) {
	switch key {
	case "ArrowUp":
		return tcell.KeyUp, 0
	case "ArrowDown":
		return tcell.KeyDown, 0
	case "ArrowLeft":
		return tcell.KeyLeft, 0
	case "ArrowRight":
		return tcell.KeyRight, 0
	case "Enter":
		return tcell.KeyEnter, 0
	case "Escape":
		return tcell.KeyEscape, 0
	case "Backspace":
		return tcell.KeyBackspace2, 0
	case "Delete":
		return tcell.KeyDelete, 0
	case "Tab":
		if shift {
			return tcell.KeyBacktab, 0
		}
		return tcell.KeyTab, 0
	case "Home":
		return tcell.KeyHome, 0
	case "End":
		return tcell.KeyEnd, 0
	}

	// Single character keys
	if len(key) == 1 {
		r := rune(key[0])
		// Ecosystem-wide policy: no Ctrl bindings anywhere. Any Ctrl+letter
		// combo is a silent no-op (returning KeyRune with ch=0 means "nothing
		// to insert"). Shift is Shift and passes through as the literal
		// character (the browser already folds it into the key string).
		if ctrl {
			return tcell.KeyRune, 0
		}
		return tcell.KeyRune, r
	}

	// Multi-character key name we don't handle (Shift, Control, etc.)
	return tcell.KeyRune, 0
}

// getVisibleRows returns structured data for all visible tree rows.
func getVisibleRows(this js.Value, args []js.Value) interface{} {
	if session == nil {
		return jsError("session not initialized")
	}
	rows := session.GetVisibleRows()
	arr := js.Global().Get("Array").New(len(rows))
	for i, row := range rows {
		obj := js.Global().Get("Object").New()
		obj.Set("name", row.Name)
		obj.Set("description", row.Description)
		obj.Set("depth", row.Depth)
		obj.Set("isFolder", row.IsFolder)
		obj.Set("isSelected", row.IsSelected)
		obj.Set("isTopMatch", row.IsTopMatch)
		obj.Set("nameMatchIndices", intsToJS(row.NameMatchIndices))
		obj.Set("descMatchIndices", intsToJS(row.DescMatchIndices))
		arr.SetIndex(i, obj)
	}
	return arr
}

// getPromptState returns structured prompt bar state.
func getPromptState(this js.Value, args []js.Value) interface{} {
	if session == nil {
		return jsError("session not initialized")
	}
	ps := session.GetPromptState()
	obj := js.Global().Get("Object").New()
	obj.Set("mode", ps.Mode)
	obj.Set("icon", ps.Icon)
	scopeArr := js.Global().Get("Array").New(len(ps.ScopePath))
	for i, s := range ps.ScopePath {
		scopeArr.SetIndex(i, s)
	}
	obj.Set("scopePath", scopeArr)
	obj.Set("query", ps.Query)
	obj.Set("cursor", ps.Cursor)
	obj.Set("ghost", ps.Ghost)
	obj.Set("hint", ps.Hint)
	return obj
}

// getUIState returns structured chrome/metadata state.
func getUIState(this js.Value, args []js.Value) interface{} {
	if session == nil {
		return jsError("session not initialized")
	}
	ui := session.GetUIState()
	obj := js.Global().Get("Object").New()
	obj.Set("title", ui.Title)
	obj.Set("titlePos", ui.TitlePos)
	obj.Set("label", ui.Label)
	obj.Set("border", ui.Border)
	obj.Set("treeOffset", ui.TreeOffset)
	obj.Set("totalVisible", ui.TotalVisible)
	return obj
}

func intsToJS(ints []int) interface{} {
	if ints == nil {
		return js.Null()
	}
	arr := js.Global().Get("Array").New(len(ints))
	for i, v := range ints {
		arr.SetIndex(i, v)
	}
	return arr
}

func frameToJS(frame tui.SessionFrame) interface{} {
	obj := js.Global().Get("Object").New()
	obj.Set("ansi", frame.ANSI)
	obj.Set("cursorX", frame.CursorX)
	obj.Set("cursorY", frame.CursorY)
	return obj
}

func jsError(msg string) interface{} {
	return js.Global().Get("Error").New(msg)
}
