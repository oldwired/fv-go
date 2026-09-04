// Package theme is the framework-wide color palette. Widgets read color
// attributes for their UI chrome from Get(); hosts can swap the active
// palette at runtime with Set(*Palette).
//
// Default is the classic Turbo Vision look (cyan dialogs, green menu
// highlights, …) and is byte-for-byte identical to what fv-go painted
// before the palette indirection was introduced. Swap it for a dark
// palette, a high-contrast palette, etc. with theme.Set(&MyPalette).
//
// The palette covers UI chrome and reusable widget surfaces. A handful
// of MakeAttr literals deliberately stay outside the palette — see
// the comments on those call sites for rationale.
package theme

import (
	"sync"

	"github.com/oldwired/fv-go/pkg/fv/types"
)

// Palette groups every semantic color role used by fv-go's standard UI
// chrome. Adding a new role: pick a descriptive name, populate it in
// Default with the legacy literal, and reference theme.Get().Role at
// the call site.
type Palette struct {
	// Frames / windows / desktop chrome
	FrameNormal       uint16 // passive window frame
	FrameActive       uint16 // focused window frame
	FrameIcons        uint16 // close / zoom / number badges
	WindowBackground  uint16 // window body fill
	WindowShadow      uint16 // drop-shadow under windows
	DesktopBackground uint16 // pattern between menu bar and status line

	// Menu bar
	MenuBarNormal       uint16 // top-level menu strip
	MenuBarHot          uint16 // hotkey letter in menu strip
	MenuItemSelected    uint16 // highlighted strip item
	MenuItemSelectedHot uint16 // hotkey letter inside the highlighted strip item

	// Menu box (drop-down)
	MenuBoxFrame            uint16 // border around the popup
	MenuBoxNormal           uint16 // popup body row
	MenuBoxNormalHot        uint16 // hotkey letter on a normal row
	MenuBoxSelected         uint16 // currently-highlighted row
	MenuBoxSelectedHot      uint16 // hotkey letter on the highlighted row
	MenuBoxDisabled         uint16 // greyed-out unselectable row
	MenuBoxDisabledSelected uint16 // disabled row when the highlight bar covers it
	MenuBoxShortcut         uint16 // dim right-aligned chord hint
	MenuBoxShortcutSelected uint16 // shortcut on the currently-highlighted row
	MenuBoxShadow           uint16 // drop-shadow under the popup

	// Status line
	StatusBarNormal uint16 // status line body
	StatusBarHot    uint16 // shortcut letter inside a status item

	// Scroll bars / lists
	ScrollBarPage   uint16 // empty trough between thumb and ends
	ScrollBarThumb  uint16 // draggable thumb + arrow buttons
	ListItemNormal  uint16 // unselected list row
	ListItemFocused uint16 // currently-highlighted list row

	// Splitter
	SplitterBar    uint16 // vertical/horizontal divider line
	SplitterHandle uint16 // draggable grip in the middle of the bar

	// Help viewer
	HelpBackground uint16 // body of the help window
	HelpHeading    uint16 // bold heading lines
	HelpBullet     uint16 // bullet markers in lists
	HelpLink       uint16 // cross-references / external links
	HelpCode       uint16 // inline code and fenced blocks
	HelpRule       uint16 // horizontal rules
	HelpQuote      uint16 // blockquote bodies

	// Dialogs
	DialogBackground uint16 // dialog body fill

	// Buttons
	ButtonNormal     uint16 // unfocused button face
	ButtonNormalHot  uint16 // hotkey letter on an unfocused button
	ButtonDefault    uint16 // default-action button face (▶ leader)
	ButtonDefaultHot uint16 // hotkey letter on the default button
	ButtonFocused    uint16 // currently-focused button face
	ButtonFocusedHot uint16 // hotkey letter on the focused button
	ButtonPressed    uint16 // button being depressed by mouse/keystroke
	ButtonPressedHot uint16 // hotkey letter while pressed
	ButtonShadow     uint16 // drop-shadow row/column

	// Input line
	InputUnfocused uint16 // text-input field at rest
	InputFocused   uint16 // text-input field with caret
	InputSelected  uint16 // selected text inside the field
	InputArrow     uint16 // ◄►/▾ affordances for history/combos

	// Cluster (radio / checkbox group)
	ClusterNormal      uint16 // item label, group unfocused
	ClusterHot         uint16 // hotkey letter, group unfocused
	ClusterFocusNormal uint16 // item label on the focused row
	ClusterFocusHot    uint16 // hotkey letter on the focused row

	// Static text / label
	LabelNormal     uint16 // label text when its linked field is unfocused
	LabelHot        uint16 // hotkey letter, link unfocused
	LabelFocused    uint16 // label text when its linked field has focus
	LabelFocusedHot uint16 // hotkey letter, link focused

	// History
	HistorySides  uint16 // frame sides of the popup
	HistoryWindow uint16 // popup body / item rows
	HistoryArrow  uint16 // ▾ dropdown affordance next to InputLine

	// Tabs
	TabActive      uint16 // currently-selected tab label
	TabActiveHot   uint16 // hotkey letter on the active tab
	TabInactive    uint16 // unselected tab label
	TabInactiveHot uint16 // hotkey letter on an inactive tab
	TabBar         uint16 // strip background
	TabSeparator   uint16 // divider line between body and strip

	// Accordion
	AccordionHeader  uint16 // collapsible section header
	AccordionExpand  uint16 // ▸/▾ expansion marker
	AccordionContent uint16 // expanded section body

	// Tree view
	TreeNormal  uint16 // unfocused row text
	TreeFocused uint16 // currently-highlighted row
	TreeBranch  uint16 // ▾/▸ expand markers and branch glyphs
	TreeIcon    uint16 // expand/collapse marker

	// Combo box
	ComboButton uint16 // ▾ button that opens the dropdown

	// Toolbar
	ToolbarNormal  uint16 // button strip at rest
	ToolbarHover   uint16 // button under the mouse cursor
	ToolbarPressed uint16 // button being clicked

	// Tooltip
	TooltipNormal uint16 // tooltip body
	TooltipShadow uint16 // drop-shadow under the tooltip

	// HoverPopup (multi-line non-modal popup)
	HoverPopupNormal uint16 // popup body text
	HoverPopupFrame  uint16 // popup border

	// Pop-up menu
	PopupMenuNormal      uint16 // unselected item row
	PopupMenuSelected    uint16 // highlighted row
	PopupMenuFrame       uint16 // border around the popup
	PopupMenuHot         uint16 // matching-filter letter on a row
	PopupMenuSelectedHot uint16 // matching-filter letter on the highlighted row

	// Fuzzy finder
	FuzzyFinderNormal    uint16 // candidate row text
	FuzzyFinderSelected  uint16 // currently-highlighted candidate
	FuzzyFinderHighlight uint16 // matched substring inside a row
	FuzzyFinderPrompt    uint16 // query prompt line
	FuzzyFinderFrame     uint16 // border around the finder

	// Breadcrumb
	BreadcrumbNormal    uint16 // non-current path segment
	BreadcrumbSeparator uint16 // ▸ between segments
	BreadcrumbCurrent   uint16 // active/last segment

	// Notification
	NotificationInfo   uint16 // info-severity toast
	NotificationWarn   uint16 // warning-severity toast
	NotificationError  uint16 // error-severity toast
	NotificationShadow uint16 // drop-shadow under the toast

	// Calendar
	CalendarFrame   uint16 // border + month/year header
	CalendarToday   uint16 // today's date cell
	CalendarFocused uint16 // currently-selected date cell
	CalendarWeekend uint16 // Sat/Sun cells
	CalendarDimmed  uint16 // leading/trailing days from adjacent months

	// Grid
	GridHeader     uint16 // column-header row
	GridHeaderSep  uint16 // separator line under the header
	GridCell       uint16 // body cell (odd row)
	GridCellAlt    uint16 // body cell (even row, zebra striping)
	GridCellCursor uint16 // cell under the cursor
	GridPinned     uint16 // frozen / pinned column tint
	GridFrame      uint16 // outer border

	// Editor / syntax / gutter
	EditorText        uint16 // body text
	EditorSelected    uint16 // selected range
	EditorKeyword     uint16 // language keyword token (if, for, …)
	EditorString      uint16 // string-literal token
	EditorComment     uint16 // comment token
	EditorNumber      uint16 // numeric-literal token
	EditorLineNo      uint16 // line-number gutter column
	EditorBookmark    uint16 // ★ gutter marker
	EditorBreakpoint  uint16 // ● gutter marker
	EditorCaretExtra  uint16 // secondary carets in multi-cursor mode
	EditorFoldSummary uint16 // "⋯ N lines" suffix on a collapsed fold header
	EditorFoldMarker  uint16 // ▸/▾ fold affordance in the gutter

	// Markdown rendering (also used by SyntaxStyles factories)
	MarkdownHeading uint16 // # H1 / ## H2 / … heading lines
	MarkdownEmph    uint16 // *emph* / _emph_
	MarkdownCode    uint16 // inline `code` and fenced blocks
	MarkdownLink    uint16 // [text](url) link target
	MarkdownStrike  uint16 // ~~strike~~
	MarkdownBullet  uint16 // - / * list bullets
	MarkdownQuote   uint16 // > blockquote body
	MarkdownRule    uint16 // --- horizontal rules
	MarkdownImage   uint16 // ![alt](url) image placeholder

	// Hex editor
	HexAddr    uint16 // left address column
	HexByte    uint16 // hex byte pairs in the middle column
	HexAscii   uint16 // ASCII gutter on the right
	HexFocused uint16 // byte under the cursor (in either column)

	// Logviewer
	LogTime  uint16 // timestamp column
	LogText  uint16 // default message text
	LogError uint16 // error-severity messages
	LogWarn  uint16 // warn-severity messages
	LogInfo  uint16 // info-severity messages

	// Progress bar / task progress
	ProgressEmpty  uint16 // unfilled portion of the bar
	ProgressFilled uint16 // filled portion of the bar
	ProgressText   uint16 // percent / count overlay text

	// Battery / charge widgets
	GaugeGood  uint16 // green normal
	GaugeWarn  uint16 // yellow warn
	GaugeCrit  uint16 // red critical
	GaugeBack  uint16 // unfilled background
	GaugeLabel uint16 // numeric overlay

	// LED digits / blinker / marquee
	LedDigit    uint16 // 7-segment digit foreground
	BlinkText   uint16 // text in the blink widget
	MarqueeText uint16 // scrolling marquee text

	// CPU / RAM / network / disk / process — generic stats
	StatHeader   uint16 // section header row
	StatLabel    uint16 // metric name column
	StatValue    uint16 // metric value column
	StatPositive uint16 // good / increasing values
	StatNegative uint16 // bad / decreasing values
	StatNeutral  uint16 // neutral / no-change values

	// Modern file dialog
	ModernFrame    uint16 // dialog border
	ModernSelected uint16 // currently-highlighted entry
	ModernFilter   uint16 // filter input line / pattern hint

	// Color selector swatches default frame
	ColorSelFrame uint16 // border around the color swatch grid

	// Hyperlink
	HyperlinkNormal  uint16 // unvisited link
	HyperlinkVisited uint16 // visited link

	// Toggle / spinner
	ToggleOn     uint16 // toggle in the "on" state
	ToggleOff    uint16 // toggle in the "off" state
	SpinnerColor uint16 // animated spinner glyph

	// Clock + heap-view system gadgets
	ClockFace  uint16 // digital text / analog face rim
	ClockHourH uint16 // analog hour hand
	ClockMinH  uint16 // analog minute hand
	ClockSecH  uint16 // analog second hand
	HeapValue  uint16 // heap-view byte counter

	// AsciiTab
	AsciiTabActive   uint16 // currently-selected ASCII-art tab
	AsciiTabInactive uint16 // unselected tab

	// Generic accent palettes for charts (used by barchart / sparkline / vumeter)
	ChartBar1 uint16 // first accent (primary series)
	ChartBar2 uint16 // second accent
	ChartBar3 uint16 // third accent
	ChartBar4 uint16 // fourth accent
	ChartAxis uint16 // axes / gridlines / tick labels
}

// Default is the classic Turbo Vision palette — cyan window bodies,
// green menu highlights, light-gray status line, etc. Every entry here
// reproduces a literal previously inlined at the call site so a fresh
// build is bit-for-bit identical to pre-theme main.
var Default = &Palette{
	FrameNormal:       types.MakeAttr(0x07, 0x03),
	FrameActive:       types.MakeAttr(0x0F, 0x03),
	FrameIcons:        types.MakeAttr(0x0E, 0x03),
	WindowBackground:  types.MakeAttr(0x07, 0x03),
	WindowShadow:      types.MakeAttr(0x08, 0x00),
	DesktopBackground: types.MakeAttr(0x07, 0x01),

	MenuBarNormal:       types.MakeAttr(0x00, 0x07),
	MenuBarHot:          types.MakeAttr(0x04, 0x07),
	MenuItemSelected:    types.MakeAttr(0x0F, 0x02),
	MenuItemSelectedHot: types.MakeAttr(0x0E, 0x02),

	MenuBoxFrame:            types.MakeAttr(0x00, 0x07),
	MenuBoxNormal:           types.MakeAttr(0x00, 0x07),
	MenuBoxNormalHot:        types.MakeAttr(0x04, 0x07),
	MenuBoxSelected:         types.MakeAttr(0x0F, 0x02),
	MenuBoxSelectedHot:      types.MakeAttr(0x0E, 0x02),
	MenuBoxDisabled:         types.MakeAttr(0x08, 0x07),
	MenuBoxDisabledSelected: types.MakeAttr(0x08, 0x02),
	MenuBoxShortcut:         types.MakeAttr(0x08, 0x07), // dark-gray on light-gray
	MenuBoxShortcutSelected: types.MakeAttr(0x07, 0x02), // light-gray on green
	MenuBoxShadow:           types.MakeAttr(0x08, 0x00),

	StatusBarNormal: types.MakeAttr(0x00, 0x07),
	StatusBarHot:    types.MakeAttr(0x04, 0x07),

	ScrollBarPage:   types.MakeAttr(0x07, 0x03),
	ScrollBarThumb:  types.MakeAttr(0x0F, 0x03),
	ListItemNormal:  types.MakeAttr(0x07, 0x00),
	ListItemFocused: types.MakeAttr(0x0F, 0x07),

	SplitterBar:    types.MakeAttr(0x00, 0x07),
	SplitterHandle: types.MakeAttr(0x0F, 0x07),

	HelpBackground: types.MakeAttr(0x00, 0x07),
	HelpHeading:    types.MakeAttr(0x04, 0x07),
	HelpBullet:     types.MakeAttr(0x01, 0x07),
	HelpLink:       types.MakeAttr(0x09, 0x07),
	HelpCode:       types.MakeAttr(0x05, 0x07),
	HelpRule:       types.MakeAttr(0x08, 0x07),
	HelpQuote:      types.MakeAttr(0x06, 0x07),

	DialogBackground: types.MakeAttr(0x00, 0x03),

	ButtonNormal:     types.MakeAttr(0x00, 0x02),
	ButtonNormalHot:  types.MakeAttr(0x0E, 0x02),
	ButtonDefault:    types.MakeAttr(0x0F, 0x02),
	ButtonDefaultHot: types.MakeAttr(0x0E, 0x02),
	ButtonFocused:    types.MakeAttr(0x00, 0x0A),
	ButtonFocusedHot: types.MakeAttr(0x01, 0x0A),
	ButtonPressed:    types.MakeAttr(0x0F, 0x01),
	ButtonPressedHot: types.MakeAttr(0x0E, 0x01),
	ButtonShadow:     types.MakeAttr(0x08, 0x03),

	InputUnfocused: types.MakeAttr(0x00, 0x07),
	InputFocused:   types.MakeAttr(0x0F, 0x01),
	InputSelected:  types.MakeAttr(0x0F, 0x06),
	InputArrow:     types.MakeAttr(0x0E, 0x06),

	ClusterNormal:      types.MakeAttr(0x07, 0x01),
	ClusterHot:         types.MakeAttr(0x0E, 0x01),
	ClusterFocusNormal: types.MakeAttr(0x0F, 0x03),
	ClusterFocusHot:    types.MakeAttr(0x0E, 0x03),

	LabelNormal:     types.MakeAttr(0x00, 0x03),
	LabelHot:        types.MakeAttr(0x0F, 0x03),
	LabelFocused:    types.MakeAttr(0x0F, 0x03),
	LabelFocusedHot: types.MakeAttr(0x0E, 0x03),

	HistorySides:  types.MakeAttr(0x00, 0x06),
	HistoryWindow: types.MakeAttr(0x07, 0x05),
	HistoryArrow:  types.MakeAttr(0x0E, 0x06),

	TabActive:      types.MakeAttr(0x0F, 0x03),
	TabActiveHot:   types.MakeAttr(0x0E, 0x03),
	TabInactive:    types.MakeAttr(0x07, 0x01),
	TabInactiveHot: types.MakeAttr(0x0E, 0x01),
	TabBar:         types.MakeAttr(0x00, 0x07),
	TabSeparator:   types.MakeAttr(0x08, 0x07),

	AccordionHeader:  types.MakeAttr(0x0F, 0x01),
	AccordionExpand:  types.MakeAttr(0x0E, 0x01),
	AccordionContent: types.MakeAttr(0x07, 0x01),

	TreeNormal:  types.MakeAttr(0x07, 0x01),
	TreeFocused: types.MakeAttr(0x0F, 0x02),
	TreeBranch:  types.MakeAttr(0x0B, 0x01),
	TreeIcon:    types.MakeAttr(0x0E, 0x01),

	ComboButton: types.MakeAttr(0x0E, 0x06),

	ToolbarNormal:  types.MakeAttr(0x00, 0x07),
	ToolbarHover:   types.MakeAttr(0x0F, 0x07),
	ToolbarPressed: types.MakeAttr(0x0F, 0x02),

	TooltipNormal: types.MakeAttr(0x00, 0x0E),
	TooltipShadow: types.MakeAttr(0x08, 0x00),

	HoverPopupNormal: types.MakeAttr(0x00, 0x0E),
	HoverPopupFrame:  types.MakeAttr(0x08, 0x0E),

	PopupMenuNormal:      types.MakeAttr(0x00, 0x07),
	PopupMenuSelected:    types.MakeAttr(0x0F, 0x02),
	PopupMenuFrame:       types.MakeAttr(0x00, 0x07),
	PopupMenuHot:         types.MakeAttr(0x04, 0x07),
	PopupMenuSelectedHot: types.MakeAttr(0x0E, 0x02),

	FuzzyFinderNormal:    types.MakeAttr(0x07, 0x01),
	FuzzyFinderSelected:  types.MakeAttr(0x0F, 0x02),
	FuzzyFinderHighlight: types.MakeAttr(0x0E, 0x01),
	FuzzyFinderPrompt:    types.MakeAttr(0x0F, 0x01),
	FuzzyFinderFrame:     types.MakeAttr(0x00, 0x07),

	BreadcrumbNormal:    types.MakeAttr(0x00, 0x07),
	BreadcrumbSeparator: types.MakeAttr(0x08, 0x07),
	BreadcrumbCurrent:   types.MakeAttr(0x0F, 0x07),

	NotificationInfo:   types.MakeAttr(0x0F, 0x01),
	NotificationWarn:   types.MakeAttr(0x00, 0x0E),
	NotificationError:  types.MakeAttr(0x0F, 0x04),
	NotificationShadow: types.MakeAttr(0x08, 0x00),

	CalendarFrame:   types.MakeAttr(0x00, 0x07),
	CalendarToday:   types.MakeAttr(0x0F, 0x02),
	CalendarFocused: types.MakeAttr(0x0F, 0x01),
	CalendarWeekend: types.MakeAttr(0x04, 0x07),
	CalendarDimmed:  types.MakeAttr(0x08, 0x07),

	GridHeader:     types.MakeAttr(0x0F, 0x01),
	GridHeaderSep:  types.MakeAttr(0x08, 0x01),
	GridCell:       types.MakeAttr(0x0F, 0x01),
	GridCellAlt:    types.MakeAttr(0x07, 0x01),
	GridCellCursor: types.MakeAttr(0x0F, 0x02),
	GridPinned:     types.MakeAttr(0x0F, 0x05),
	GridFrame:      types.MakeAttr(0x00, 0x07),

	EditorText:        types.MakeAttr(0x07, 0x01),
	EditorSelected:    types.MakeAttr(0x0F, 0x02),
	EditorKeyword:     types.MakeAttr(0x0E, 0x01),
	EditorString:      types.MakeAttr(0x0A, 0x01),
	EditorComment:     types.MakeAttr(0x08, 0x01),
	EditorNumber:      types.MakeAttr(0x0B, 0x01),
	EditorLineNo:      types.MakeAttr(0x08, 0x00),
	EditorBookmark:    types.MakeAttr(0x0E, 0x00),
	EditorBreakpoint:  types.MakeAttr(0x0C, 0x00),
	EditorCaretExtra:  types.MakeAttr(0x01, 0x07),
	EditorFoldSummary: types.MakeAttr(0x08, 0x01),
	EditorFoldMarker:  types.MakeAttr(0x0E, 0x00),

	MarkdownHeading: types.MakeAttr(0x0E, 0x01),
	MarkdownEmph:    types.MakeAttr(0x0F, 0x01),
	MarkdownCode:    types.MakeAttr(0x0B, 0x01),
	MarkdownLink:    types.MakeAttr(0x09, 0x01),
	MarkdownStrike:  types.MakeAttr(0x08, 0x01), // dimmed strike-through (no underline in cell model — color cues it)
	MarkdownBullet:  types.MakeAttr(0x0B, 0x01),
	MarkdownQuote:   types.MakeAttr(0x06, 0x01),
	MarkdownRule:    types.MakeAttr(0x08, 0x01),
	MarkdownImage:   types.MakeAttr(0x0D, 0x01),

	HexAddr:    types.MakeAttr(0x07, 0x01),
	HexByte:    types.MakeAttr(0x0E, 0x01),
	HexAscii:   types.MakeAttr(0x0F, 0x01),
	HexFocused: types.MakeAttr(0x0B, 0x01),

	LogTime:  types.MakeAttr(0x06, 0x00),
	LogText:  types.MakeAttr(0x07, 0x00),
	LogError: types.MakeAttr(0x0C, 0x00),
	LogWarn:  types.MakeAttr(0x0E, 0x00),
	LogInfo:  types.MakeAttr(0x0F, 0x00),

	ProgressEmpty:  types.MakeAttr(0x07, 0x01),
	ProgressFilled: types.MakeAttr(0x0E, 0x02),
	ProgressText:   types.MakeAttr(0x0F, 0x00),

	GaugeGood:  types.MakeAttr(0x0A, 0x00),
	GaugeWarn:  types.MakeAttr(0x0E, 0x00),
	GaugeCrit:  types.MakeAttr(0x0C, 0x00),
	GaugeBack:  types.MakeAttr(0x08, 0x00),
	GaugeLabel: types.MakeAttr(0x0F, 0x00),

	LedDigit:    types.MakeAttr(0x0C, 0x00),
	BlinkText:   types.MakeAttr(0x0E, 0x00),
	MarqueeText: types.MakeAttr(0x0F, 0x01),

	StatHeader:   types.MakeAttr(0x0F, 0x01),
	StatLabel:    types.MakeAttr(0x07, 0x00),
	StatValue:    types.MakeAttr(0x0F, 0x00),
	StatPositive: types.MakeAttr(0x0A, 0x00),
	StatNegative: types.MakeAttr(0x0C, 0x00),
	StatNeutral:  types.MakeAttr(0x07, 0x00),

	ModernFrame:    types.MakeAttr(0x00, 0x07),
	ModernSelected: types.MakeAttr(0x0F, 0x02),
	ModernFilter:   types.MakeAttr(0x00, 0x07),

	ColorSelFrame: types.MakeAttr(0x00, 0x07),

	HyperlinkNormal:  types.MakeAttr(0x09, 0x00),
	HyperlinkVisited: types.MakeAttr(0x05, 0x00),

	ToggleOn:     types.MakeAttr(0x0E, 0x03),
	ToggleOff:    types.MakeAttr(0x08, 0x03),
	SpinnerColor: types.MakeAttr(0x0E, 0x00),

	// Clock palette: white face on default bg, yellow hour, cyan minute,
	// bright red second hand. Echoes a vintage Borland desktop clock.
	ClockFace:  types.MakeAttr(0x0F, 0x00),
	ClockHourH: types.MakeAttr(0x0E, 0x00),
	ClockMinH:  types.MakeAttr(0x0B, 0x00),
	ClockSecH:  types.MakeAttr(0x0C, 0x00),
	HeapValue:  types.MakeAttr(0x0E, 0x07),

	AsciiTabActive:   types.MakeAttr(0x0F, 0x03),
	AsciiTabInactive: types.MakeAttr(0x07, 0x01),

	ChartBar1: types.MakeAttr(0x0A, 0x00),
	ChartBar2: types.MakeAttr(0x0B, 0x00),
	ChartBar3: types.MakeAttr(0x0E, 0x00),
	ChartBar4: types.MakeAttr(0x0C, 0x00),
	ChartAxis: types.MakeAttr(0x08, 0x00),
}

var (
	mu     sync.RWMutex
	active *Palette = Default
	onChg  func()
)

// Get returns the currently active palette. Callers should cache the
// returned pointer for the duration of a single Draw — both to avoid
// repeated mutex traffic and to ensure a single frame sees a consistent
// snapshot.
func Get() *Palette {
	mu.RLock()
	defer mu.RUnlock()
	return active
}

// Set swaps the active palette and fires the registered onChange hook
// (typically views.MarkDirty wired by app.NewProgram). Passing nil
// reverts to Default.
func Set(p *Palette) {
	if p == nil {
		p = Default
	}
	mu.Lock()
	active = p
	cb := onChg
	mu.Unlock()
	if cb != nil {
		cb()
	}
}

// SetOnChange installs the callback invoked from Set(). The app layer
// wires this to views.MarkDirty so a palette swap triggers a repaint
// without callers needing to know about the views package.
func SetOnChange(f func()) {
	mu.Lock()
	onChg = f
	mu.Unlock()
}
