// Package consts defines the integer constants used throughout fv-go:
// type IDs, command IDs, help contexts, and history list IDs.
//
// Ported from Delphi unit fvconsts.pas. Names follow the Pascal originals
// (cmOK, hcOk, idView, ...) so cross-referencing the FV-Delphi source
// stays trivial.
package consts

// View / dialog / app type IDs (used by serial.Registry).
const (
	IDView       = 1
	IDFrame      = 2
	IDScrollBar  = 3
	IDScroller   = 4
	IDListViewer = 5
	IDGroup      = 6
	IDWindow     = 7

	IDDialog       = 10
	IDInputLine    = 11
	IDButton       = 12
	IDCluster      = 13
	IDRadioButtons = 14
	IDCheckBoxes   = 15
	IDListBox      = 16
	IDStaticText   = 17
	IDLabel        = 18
	IDHistory      = 19
	IDParamText    = 20

	IDBackground = 30
	IDDesktop    = 31

	IDMenuBar    = 40
	IDMenuBox    = 41
	IDStatusLine = 42
	IDMenuPopup  = 43

	IDStringList = 52

	IDPXPictureValidator    = 80
	IDFilterValidator       = 81
	IDRangeValidator        = 82
	IDStringLookupValidator = 83
)

// Command IDs. Each lives in a numeric band by origin unit (App, Views,
// Editors, ...). Two cmQuit-equivalents exist: cmQuit (1) and cmQuitApp
// (57). Match Pascal's intent — cmQuit terminates the modal loop, while
// cmQuitApp terminates the program.
const (
	// Views / dialogs
	CmValid   uint16 = 0
	CmQuit    uint16 = 1
	CmError   uint16 = 2
	CmMenu    uint16 = 3
	CmClose   uint16 = 4
	CmZoom    uint16 = 5
	CmResize  uint16 = 6
	CmNext    uint16 = 7
	CmPrev    uint16 = 8
	CmHelp    uint16 = 9
	CmOK      uint16 = 10
	CmCancel  uint16 = 11
	CmYes     uint16 = 12
	CmNo      uint16 = 13
	CmDefault uint16 = 14

	CmCut   uint16 = 20
	CmCopy  uint16 = 21
	CmPaste uint16 = 22
	CmUndo  uint16 = 23
	CmClear uint16 = 24

	CmTile           uint16 = 25
	CmCascade        uint16 = 26
	CmHide           uint16 = 27
	CmTileHorizontal uint16 = 28
	CmTileVertical   uint16 = 29

	CmReceivedFocus     uint16 = 50
	CmReleasedFocus     uint16 = 51
	CmCommandSetChanged uint16 = 52
	CmScrollBarChanged  uint16 = 53
	CmScrollBarClicked  uint16 = 54
	CmSelectWindowNum   uint16 = 55
	CmListItemSelected  uint16 = 56

	// App
	CmNew           uint16 = 30
	CmOpen          uint16 = 31
	CmSave          uint16 = 32
	CmSaveAs        uint16 = 33
	CmSaveAll       uint16 = 34
	CmChangeDir     uint16 = 35
	CmDosShell      uint16 = 36
	CmCloseAll      uint16 = 37
	CmDelete        uint16 = 38
	CmEdit          uint16 = 40
	CmAbout         uint16 = 41
	CmDesktopLoad   uint16 = 42
	CmDesktopStore  uint16 = 43
	CmNewDesktop    uint16 = 44
	CmNewMenuBar    uint16 = 45
	CmNewStatusLine uint16 = 46
	CmTransfer      uint16 = 48
	CmResizeApp     uint16 = 49
	CmQuitApp       uint16 = 57

	CmRecordHistory  uint16 = 60
	CmGrabDefault    uint16 = 61
	CmReleaseDefault uint16 = 62

	// Editor-issued commands. CmEditorGoto: emitted on Ctrl+G so a
	// host can pop a jump-to-line dialog. Tied to the editor that
	// fired it via ev.InfoPtr.
	CmEditorGoto uint16 = 63
)

// Help-context IDs used by the status line and dialogs.
const (
	HcNoContext = 0
	HcDragging  = 1
	HcOk        = 2
	HcCancel    = 3
	HcEdit      = 4
	HcDelete    = 5
	HcInsert    = 6

	HcExit = 65288
)

// History list IDs.
const (
	HiConfig             = 1
	HiDirectories        = 2
	HiDesktop            = 3
	HiCurrentDirectories = 1
	HiFiles              = 4
)

// LowAscii global flag mirroring fvconsts.LowAscii. When true, the
// frame / scrollbar / menu drawing code emits ASCII-only fallbacks
// instead of box-drawing glyphs.
var LowAscii bool
