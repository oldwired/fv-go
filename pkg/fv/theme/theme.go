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
	MenuItemSelectedHot uint16

	// Menu box (drop-down)
	MenuBoxFrame            uint16
	MenuBoxNormal           uint16
	MenuBoxNormalHot        uint16
	MenuBoxSelected         uint16
	MenuBoxSelectedHot      uint16
	MenuBoxDisabled         uint16
	MenuBoxDisabledSelected uint16
	MenuBoxShortcut         uint16 // dim right-aligned chord hint
	MenuBoxShortcutSelected uint16 // shortcut on the currently-highlighted row
	MenuBoxShadow           uint16

	// Status line
	StatusBarNormal uint16
	StatusBarHot    uint16

	// Scroll bars / lists
	ScrollBarPage   uint16
	ScrollBarThumb  uint16
	ListItemNormal  uint16
	ListItemFocused uint16

	// Splitter
	SplitterBar    uint16
	SplitterHandle uint16

	// Help viewer
	HelpBackground uint16
	HelpHeading    uint16
	HelpBullet     uint16
	HelpLink       uint16
	HelpCode       uint16
	HelpRule       uint16
	HelpQuote      uint16

	// Dialogs
	DialogBackground uint16

	// Buttons
	ButtonNormal     uint16
	ButtonNormalHot  uint16
	ButtonDefault    uint16
	ButtonDefaultHot uint16
	ButtonFocused    uint16
	ButtonFocusedHot uint16
	ButtonPressed    uint16
	ButtonPressedHot uint16
	ButtonShadow     uint16

	// Input line
	InputUnfocused uint16
	InputFocused   uint16
	InputSelected  uint16
	InputArrow     uint16

	// Cluster (radio / checkbox group)
	ClusterNormal      uint16
	ClusterHot         uint16
	ClusterFocusNormal uint16
	ClusterFocusHot    uint16

	// Static text / label
	LabelNormal     uint16
	LabelHot        uint16
	LabelFocused    uint16
	LabelFocusedHot uint16

	// History
	HistorySides  uint16
	HistoryWindow uint16
	HistoryArrow  uint16

	// Tabs
	TabActive      uint16
	TabActiveHot   uint16
	TabInactive    uint16
	TabInactiveHot uint16
	TabBar         uint16
	TabSeparator   uint16

	// Accordion
	AccordionHeader  uint16
	AccordionExpand  uint16
	AccordionContent uint16

	// Tree view
	TreeNormal  uint16
	TreeFocused uint16
	TreeIcon    uint16

	// Combo box
	ComboButton uint16

	// Toolbar
	ToolbarNormal  uint16
	ToolbarHover   uint16
	ToolbarPressed uint16

	// Tooltip
	TooltipNormal uint16
	TooltipShadow uint16

	// Pop-up menu
	PopupMenuNormal      uint16
	PopupMenuSelected    uint16
	PopupMenuFrame       uint16
	PopupMenuHot         uint16
	PopupMenuSelectedHot uint16

	// Fuzzy finder
	FuzzyFinderNormal    uint16
	FuzzyFinderSelected  uint16
	FuzzyFinderHighlight uint16
	FuzzyFinderPrompt    uint16
	FuzzyFinderFrame     uint16

	// Breadcrumb
	BreadcrumbNormal    uint16
	BreadcrumbSeparator uint16
	BreadcrumbCurrent   uint16

	// Notification
	NotificationInfo   uint16
	NotificationWarn   uint16
	NotificationError  uint16
	NotificationShadow uint16

	// Calendar
	CalendarFrame   uint16
	CalendarToday   uint16
	CalendarFocused uint16
	CalendarWeekend uint16
	CalendarDimmed  uint16

	// Grid
	GridHeader     uint16
	GridHeaderSep  uint16
	GridCell       uint16
	GridCellAlt    uint16
	GridCellCursor uint16
	GridPinned     uint16
	GridFrame      uint16

	// Editor / syntax / gutter
	EditorText       uint16
	EditorSelected   uint16
	EditorKeyword    uint16
	EditorString     uint16
	EditorComment    uint16
	EditorNumber     uint16
	EditorLineNo     uint16
	EditorBookmark   uint16
	EditorBreakpoint uint16

	// Markdown rendering (also used by SyntaxStyles factories)
	MarkdownHeading uint16
	MarkdownEmph    uint16
	MarkdownCode    uint16
	MarkdownLink    uint16
	MarkdownStrike  uint16
	MarkdownBullet  uint16
	MarkdownQuote   uint16
	MarkdownRule    uint16
	MarkdownImage   uint16

	// Hex editor
	HexAddr    uint16
	HexByte    uint16
	HexAscii   uint16
	HexFocused uint16

	// Logviewer
	LogTime  uint16
	LogText  uint16
	LogError uint16
	LogWarn  uint16
	LogInfo  uint16

	// Progress bar / task progress
	ProgressEmpty  uint16
	ProgressFilled uint16
	ProgressText   uint16

	// Battery / charge widgets
	GaugeGood  uint16 // green normal
	GaugeWarn  uint16 // yellow warn
	GaugeCrit  uint16 // red critical
	GaugeBack  uint16 // unfilled background
	GaugeLabel uint16 // numeric overlay

	// LED digits / blinker / marquee
	LedDigit    uint16
	BlinkText   uint16
	MarqueeText uint16

	// CPU / RAM / network / disk / process — generic stats
	StatHeader   uint16
	StatLabel    uint16
	StatValue    uint16
	StatPositive uint16
	StatNegative uint16
	StatNeutral  uint16

	// Modern file dialog
	ModernFrame    uint16
	ModernSelected uint16
	ModernFilter   uint16

	// Color selector swatches default frame
	ColorSelFrame uint16

	// Hyperlink
	HyperlinkNormal  uint16
	HyperlinkVisited uint16

	// Toggle / spinner
	ToggleOn     uint16
	ToggleOff    uint16
	SpinnerColor uint16

	// Clock + heap-view system gadgets
	ClockFace  uint16 // digital text / analog face rim
	ClockHourH uint16 // analog hour hand
	ClockMinH  uint16 // analog minute hand
	ClockSecH  uint16 // analog second hand
	HeapValue  uint16 // heap-view byte counter

	// AsciiTab
	AsciiTabActive   uint16
	AsciiTabInactive uint16

	// Generic accent palettes for charts (used by barchart / sparkline / vumeter)
	ChartBar1 uint16
	ChartBar2 uint16
	ChartBar3 uint16
	ChartBar4 uint16
	ChartAxis uint16
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
	ButtonFocused:    types.MakeAttr(0x0F, 0x0A),
	ButtonFocusedHot: types.MakeAttr(0x0E, 0x0A),
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
	TreeIcon:    types.MakeAttr(0x0E, 0x01),

	ComboButton: types.MakeAttr(0x0E, 0x06),

	ToolbarNormal:  types.MakeAttr(0x00, 0x07),
	ToolbarHover:   types.MakeAttr(0x0F, 0x07),
	ToolbarPressed: types.MakeAttr(0x0F, 0x02),

	TooltipNormal: types.MakeAttr(0x00, 0x0E),
	TooltipShadow: types.MakeAttr(0x08, 0x00),

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

	EditorText:       types.MakeAttr(0x07, 0x01),
	EditorSelected:   types.MakeAttr(0x0F, 0x02),
	EditorKeyword:    types.MakeAttr(0x0E, 0x01),
	EditorString:     types.MakeAttr(0x0A, 0x01),
	EditorComment:    types.MakeAttr(0x08, 0x01),
	EditorNumber:     types.MakeAttr(0x0B, 0x01),
	EditorLineNo:     types.MakeAttr(0x08, 0x00),
	EditorBookmark:   types.MakeAttr(0x0E, 0x00),
	EditorBreakpoint: types.MakeAttr(0x0C, 0x00),

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
