package consts

// Key codes ported from Drivers.pas. The legacy Pascal encoding stores
// the IBM scan code in the high byte and the ASCII char in the low byte.
// We preserve those exact uint16 values so existing FV-style apps and
// data files (menus, hotkey strings, .res files) port verbatim.
const (
	KbNoKey uint16 = 0x0000

	KbAltEsc   uint16 = 0x0100
	KbEsc      uint16 = 0x011B
	KbAltSpace uint16 = 0x0200

	KbCtrlIns  uint16 = 0x0400
	KbShiftIns uint16 = 0x0500
	KbCtrlDel  uint16 = 0x0600
	KbShiftDel uint16 = 0x0700

	KbAltBack      uint16 = 0x0800
	KbAltShiftBack uint16 = 0x0900
	KbBack         uint16 = 0x0E08
	KbCtrlBack     uint16 = 0x0E7F

	KbShiftTab uint16 = 0x0F00
	KbTab      uint16 = 0x0F09

	KbAltQ uint16 = 0x1000
	KbAltW uint16 = 0x1100
	KbAltE uint16 = 0x1200
	KbAltR uint16 = 0x1300
	KbAltT uint16 = 0x1400
	KbAltY uint16 = 0x1500
	KbAltU uint16 = 0x1600
	KbAltI uint16 = 0x1700
	KbAltO uint16 = 0x1800
	KbAltP uint16 = 0x1900
	KbAltA uint16 = 0x1E00
	KbAltS uint16 = 0x1F00
	KbAltD uint16 = 0x2000
	KbAltF uint16 = 0x2100
	KbAltG uint16 = 0x2200
	KbAltH uint16 = 0x2300
	KbAltJ uint16 = 0x2400
	KbAltK uint16 = 0x2500
	KbAltL uint16 = 0x2600
	KbAltZ uint16 = 0x2C00
	KbAltX uint16 = 0x2D00
	KbAltC uint16 = 0x2E00
	KbAltV uint16 = 0x2F00
	KbAltB uint16 = 0x3000
	KbAltN uint16 = 0x3100
	KbAltM uint16 = 0x3200

	KbCtrlA uint16 = 0x1E01
	KbCtrlB uint16 = 0x3002
	KbCtrlC uint16 = 0x2E03
	KbCtrlD uint16 = 0x2004
	KbCtrlE uint16 = 0x1205
	KbCtrlF uint16 = 0x2106
	KbCtrlG uint16 = 0x2207
	KbCtrlH uint16 = 0x2308
	KbCtrlI uint16 = 0x1709
	KbCtrlJ uint16 = 0x240A
	KbCtrlK uint16 = 0x250B
	KbCtrlL uint16 = 0x260C
	KbCtrlM uint16 = 0x320D
	KbCtrlN uint16 = 0x310E
	KbCtrlO uint16 = 0x180F
	KbCtrlP uint16 = 0x1910
	KbCtrlQ uint16 = 0x1011
	KbCtrlR uint16 = 0x1312
	KbCtrlS uint16 = 0x1F13
	KbCtrlT uint16 = 0x1414
	KbCtrlU uint16 = 0x1615
	KbCtrlV uint16 = 0x2F16
	KbCtrlW uint16 = 0x1117
	KbCtrlX uint16 = 0x2D18
	KbCtrlY uint16 = 0x1519
	KbCtrlZ uint16 = 0x2C1A

	KbCtrlEnter uint16 = 0x1C0A
	KbEnter     uint16 = 0x1C0D
	KbSpaceBar  uint16 = 0x3920

	KbF1  uint16 = 0x3B00
	KbF2  uint16 = 0x3C00
	KbF3  uint16 = 0x3D00
	KbF4  uint16 = 0x3E00
	KbF5  uint16 = 0x3F00
	KbF6  uint16 = 0x4000
	KbF7  uint16 = 0x4100
	KbF8  uint16 = 0x4200
	KbF9  uint16 = 0x4300
	KbF10 uint16 = 0x4400
	KbF11 uint16 = 0x8500
	KbF12 uint16 = 0x8600

	KbHome   uint16 = 0x4700
	KbUp     uint16 = 0x4800
	KbPgUp   uint16 = 0x4900
	KbLeft   uint16 = 0x4B00
	KbCenter uint16 = 0x4C00
	KbRight  uint16 = 0x4D00
	KbEnd    uint16 = 0x4F00
	KbDown   uint16 = 0x5000
	KbPgDn   uint16 = 0x5100
	KbIns    uint16 = 0x5200
	KbDel    uint16 = 0x5300

	KbShiftF1  uint16 = 0x5400
	KbShiftF2  uint16 = 0x5500
	KbShiftF3  uint16 = 0x5600
	KbShiftF4  uint16 = 0x5700
	KbShiftF5  uint16 = 0x5800
	KbShiftF6  uint16 = 0x5900
	KbShiftF7  uint16 = 0x5A00
	KbShiftF8  uint16 = 0x5B00
	KbShiftF9  uint16 = 0x5C00
	KbShiftF10 uint16 = 0x5D00

	KbCtrlF1  uint16 = 0x5E00
	KbCtrlF2  uint16 = 0x5F00
	KbCtrlF3  uint16 = 0x6000
	KbCtrlF4  uint16 = 0x6100
	KbCtrlF5  uint16 = 0x6200
	KbCtrlF6  uint16 = 0x6300
	KbCtrlF7  uint16 = 0x6400
	KbCtrlF8  uint16 = 0x6500
	KbCtrlF9  uint16 = 0x6600
	KbCtrlF10 uint16 = 0x6700

	KbAltF1  uint16 = 0x6800
	KbAltF2  uint16 = 0x6900
	KbAltF3  uint16 = 0x6A00
	KbAltF4  uint16 = 0x6B00
	KbAltF5  uint16 = 0x6C00
	KbAltF6  uint16 = 0x6D00
	KbAltF7  uint16 = 0x6E00
	KbAltF8  uint16 = 0x6F00
	KbAltF9  uint16 = 0x7000
	KbAltF10 uint16 = 0x7100

	KbCtrlLeft  uint16 = 0x7300
	KbCtrlRight uint16 = 0x7400
	KbCtrlEnd   uint16 = 0x7500
	KbCtrlPgDn  uint16 = 0x7600
	KbCtrlHome  uint16 = 0x7700
	KbCtrlPgUp  uint16 = 0x8400
	KbCtrlUp    uint16 = 0x8D00
	KbCtrlDown  uint16 = 0x9100
	KbCtrlTab   uint16 = 0x9400

	KbAlt0     uint16 = 0x8100
	KbAlt1     uint16 = 0x7800
	KbAlt2     uint16 = 0x7900
	KbAlt3     uint16 = 0x7A00
	KbAlt4     uint16 = 0x7B00
	KbAlt5     uint16 = 0x7C00
	KbAlt6     uint16 = 0x7D00
	KbAlt7     uint16 = 0x7E00
	KbAlt8     uint16 = 0x7F00
	KbAlt9     uint16 = 0x8000
	KbAltMinus uint16 = 0x8200
	KbAltEqual uint16 = 0x8300
	KbAltTab   uint16 = 0xA500
	KbAltHome  uint16 = 0x9700
	KbAltUp    uint16 = 0x9800
	KbAltPgUp  uint16 = 0x9900
	KbAltLeft  uint16 = 0x9B00
	KbAltRight uint16 = 0x9D00
	KbAltEnd   uint16 = 0x9F00
	KbAltDown  uint16 = 0xA000
	KbAltPgDn  uint16 = 0xA100
	KbAltIns   uint16 = 0xA200
	KbAltDel   uint16 = 0xA300
)

// Keyboard shift-state bits (Drivers.pas: GetShiftState result).
const (
	KbRightShift uint16 = 0x0001
	KbLeftShift  uint16 = 0x0002
	KbCtrlShift  uint16 = 0x0004
	KbAltShift   uint16 = 0x0008
	KbScrollLock uint16 = 0x0010
	KbNumLock    uint16 = 0x0020
	KbCapsLock   uint16 = 0x0040
	KbInsState   uint16 = 0x0080
	KbBothShifts        = KbRightShift | KbLeftShift
)

// Event-type flags (Drivers.pas: TEvent.What).
const (
	EvNothing uint16 = 0x0000

	EvMouseDown uint16 = 0x0001
	EvMouseUp   uint16 = 0x0002
	EvMouseMove uint16 = 0x0004
	EvMouseAuto uint16 = 0x0008
	EvMouse     uint16 = 0x000F

	EvKeyDown  uint16 = 0x0010
	EvKeyUp    uint16 = 0x0020
	EvKeyboard uint16 = 0x0030

	EvCommand   uint16 = 0x0100
	EvBroadcast uint16 = 0x0200
	EvMessage   uint16 = 0xFF00

	EvTerminal uint16 = 0x1000
)

// Mouse-button flags (Drivers.pas: TEvent.Buttons).
const (
	MbLeftButton      byte = 0x01
	MbRightButton     byte = 0x02
	MbMiddleButton    byte = 0x04
	MbScrollWheelDown byte = 0x08
	MbScrollWheelUp   byte = 0x10
)

// View state flags (Views.pas: TView.State).
const (
	SfVisible   uint16 = 0x0001
	SfCursorVis uint16 = 0x0002
	SfCursorIns uint16 = 0x0004
	SfShadow    uint16 = 0x0008
	SfActive    uint16 = 0x0010
	SfSelected  uint16 = 0x0020
	SfFocused   uint16 = 0x0040
	SfDragging  uint16 = 0x0080
	SfDisabled  uint16 = 0x0100
	SfModal     uint16 = 0x0200
	SfDefault   uint16 = 0x0400
	SfExposed   uint16 = 0x0800
)

// View option flags (Views.pas: TView.Options).
const (
	OfSelectable  uint16 = 0x0001
	OfTopSelect   uint16 = 0x0002
	OfFirstClick  uint16 = 0x0004
	OfFramed      uint16 = 0x0008
	OfPreProcess  uint16 = 0x0010
	OfPostProcess uint16 = 0x0020
	OfBuffered    uint16 = 0x0040
	OfTileable    uint16 = 0x0080
	OfCenterX     uint16 = 0x0100
	OfCenterY     uint16 = 0x0200
	OfCentered    uint16 = 0x0300
	OfValidate    uint16 = 0x0400
)

// Grow-mode flags (Views.pas: TView.GrowMode).
const (
	GfGrowLoX byte = 0x01
	GfGrowLoY byte = 0x02
	GfGrowHiX byte = 0x04
	GfGrowHiY byte = 0x08
	GfGrowAll byte = 0x0F
	GfGrowRel byte = 0x10
)

// Drag-mode flags (Views.pas: TView.DragMode).
const (
	DmDragMove byte = 0x01
	DmDragGrow byte = 0x02
	DmLimitLoX byte = 0x10
	DmLimitLoY byte = 0x20
	DmLimitHiX byte = 0x40
	DmLimitHiY byte = 0x80
	DmLimitAll      = DmLimitLoX | DmLimitLoY | DmLimitHiX | DmLimitHiY
)

// Window flags (Views.pas: TWindow.Flags).
const (
	WfMove   byte = 0x01
	WfGrow   byte = 0x02
	WfClose  byte = 0x04
	WfZoom   byte = 0x08
	WfNoMenu byte = 0x10
)

// Window number convenience.
const WnNoNumber = 0

// Frame icon column offsets used by TFrame.Draw.
const (
	FpFramePassive  byte = 0
	FpFrameActive   byte = 1
	FpFrameIcon     byte = 2
	FpScrollerPage  byte = 3
	FpScrollerIcons byte = 4
	FpReserved      byte = 5
)
