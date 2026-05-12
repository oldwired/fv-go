// Package hexedit provides HexEditor — a 16-bytes-per-row hex+ASCII
// viewer/editor with two edit modes (hex nibble / ASCII char), a
// caret, modified-byte tracking, and pluggable data sources.
//
// Ported from HexEdit.pas. The Pascal version uses a TBytes buffer
// backed by an interface IHexDataSource; we expose the same shape.
package hexedit

import (
	"fmt"
	"os"
	"sync"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/screen"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// BytesPerRow is fixed at 16, matching the Pascal layout.
const BytesPerRow = 16

// EditMode picks which column the cursor is editing in.
type EditMode int

const (
	ModeHex   EditMode = iota // editing in the hex column (nibbles)
	ModeASCII                 // editing in the ASCII column
)

// DataSource is the byte-array facade the editor reads/writes through.
// Implement this if you want to back the editor with something other
// than the in-memory MemorySource (e.g. an mmap'd file, a remote
// resource, a network capture buffer).
type DataSource interface {
	Size() int64
	GetByte(pos int64) byte
	SetByte(pos int64, b byte) error
	CanWrite() bool
	IsByteModified(pos int64) bool
	IsModified() bool
	ClearModified()
	// Reload discards all in-memory edits and re-reads from the
	// underlying storage. Sources that aren't file-backed should
	// return a meaningful error rather than silently no-op.
	Reload() error
}

// MemorySource is a DataSource backed by a Go byte slice. Tracks which
// positions have been modified relative to the initial load, so the
// editor can highlight them.
type MemorySource struct {
	mu       sync.RWMutex
	data     []byte
	modified map[int64]bool
	readOnly bool
	// path is set by LoadFile; Reload re-reads from it. Empty for
	// sources created via NewMemorySource directly.
	path string
}

// NewMemorySource constructs a source seeded from data. The slice is
// retained, not copied — pass a fresh slice if you don't want
// mutations to propagate to the caller.
func NewMemorySource(data []byte) *MemorySource {
	return &MemorySource{data: data, modified: map[int64]bool{}}
}

// LoadFile reads the entire file into a new MemorySource. The path
// is remembered so Reload re-reads from it without the caller having
// to thread it through.
func LoadFile(path string) (*MemorySource, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s := NewMemorySource(b)
	s.path = path
	return s, nil
}

// Reload re-reads the file the source was originally loaded from,
// discarding all unsaved changes. Returns an error if the source
// wasn't created via LoadFile (no path to re-read).
func (m *MemorySource) Reload() error {
	m.mu.Lock()
	path := m.path
	m.mu.Unlock()
	if path == "" {
		return fmt.Errorf("hexedit: source isn't file-backed; cannot reload")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.data = b
	m.modified = map[int64]bool{}
	m.mu.Unlock()
	return nil
}

// SaveFile writes the source's bytes to path and clears modified state.
func (m *MemorySource) SaveFile(path string) error {
	m.mu.RLock()
	data := append([]byte(nil), m.data...)
	m.mu.RUnlock()
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}
	m.ClearModified()
	return nil
}

// Bytes returns a copy of the underlying slice.
func (m *MemorySource) Bytes() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]byte(nil), m.data...)
}

// SetReadOnly toggles writability.
func (m *MemorySource) SetReadOnly(ro bool) { m.readOnly = ro }

// Size implements DataSource.
func (m *MemorySource) Size() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return int64(len(m.data))
}

// GetByte implements DataSource. Out-of-range positions return 0.
func (m *MemorySource) GetByte(pos int64) byte {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if pos < 0 || pos >= int64(len(m.data)) {
		return 0
	}
	return m.data[pos]
}

// SetByte implements DataSource. Returns an error if the source is
// read-only or the position is out of range.
func (m *MemorySource) SetByte(pos int64, b byte) error {
	if m.readOnly {
		return fmt.Errorf("hexedit: source is read-only")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if pos < 0 || pos >= int64(len(m.data)) {
		return fmt.Errorf("hexedit: position %d out of range", pos)
	}
	if m.data[pos] != b {
		m.data[pos] = b
		m.modified[pos] = true
	}
	return nil
}

// CanWrite implements DataSource.
func (m *MemorySource) CanWrite() bool { return !m.readOnly }

// IsByteModified implements DataSource.
func (m *MemorySource) IsByteModified(pos int64) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.modified[pos]
}

// IsModified reports whether any byte was modified.
func (m *MemorySource) IsModified() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.modified) > 0
}

// ClearModified resets the modified set.
func (m *MemorySource) ClearModified() {
	m.mu.Lock()
	m.modified = map[int64]bool{}
	m.mu.Unlock()
}

// HexEditor is the view. It scrolls vertically (16 bytes per row),
// holds a cursor at byte position Position, and toggles between hex
// and ASCII edit modes with Tab.
type HexEditor struct {
	views.Base

	Source     DataSource
	Position   int64    // current byte position
	Top        int64    // first visible row index (in rows, not bytes)
	Mode       EditMode // hex vs ASCII
	HighNibble bool     // true while editing the high nibble of a hex byte
}

// New constructs a HexEditor with bounds and initial source.
func New(bounds geom.Rect, src DataSource) *HexEditor {
	h := &HexEditor{Base: views.NewBase(bounds), Source: src, HighNibble: true}
	h.SetSelf(h)
	h.Options |= consts.OfSelectable | consts.OfFirstClick
	h.State |= consts.SfCursorVis
	h.GrowMode = consts.GfGrowHiX | consts.GfGrowHiY
	return h
}

// GetTypeID for serial registry.
func (h *HexEditor) GetTypeID() string { return "hexeditor" }

// rows returns the total number of 16-byte rows in the source.
func (h *HexEditor) rows() int64 {
	if h.Source == nil {
		return 0
	}
	s := h.Source.Size()
	return (s + BytesPerRow - 1) / BytesPerRow
}

// addressWidth returns the number of cells the address column spans.
const addressWidth = 10

// hexColStart returns the column where the hex byte at offset 0..15
// of a row begins. There's a gap after byte 7.
func hexColStart(byteIdx int) int {
	x := addressWidth + byteIdx*3
	if byteIdx >= 8 {
		x++ // extra gap between bytes 7 and 8
	}
	return x
}

// asciiColStart returns the column where ASCII rendering starts.
func asciiColStart() int { return addressWidth + 16*3 + 2 }

// SetPosition clamps p to [0, Size) and scrolls if needed.
func (h *HexEditor) SetPosition(p int64) {
	if h.Source == nil {
		return
	}
	if p < 0 {
		p = 0
	}
	if p >= h.Source.Size() {
		p = h.Source.Size() - 1
		if p < 0 {
			p = 0
		}
	}
	h.Position = p
	row := p / BytesPerRow
	visibleRows := int64(h.Size.Y)
	if row < h.Top {
		h.Top = row
	} else if row >= h.Top+visibleRows {
		h.Top = row - visibleRows + 1
	}
	h.HighNibble = true
}

// Draw paints visible rows.
func (h *HexEditor) Draw() {
	if h.Source == nil {
		return
	}
	addrColor := types.MakeAttr(0x07, 0x01)
	hexColor := types.MakeAttr(0x0E, 0x01)
	hexFocus := types.MakeAttr(0x0B, 0x01)
	cursorColor := types.MakeAttr(0x0F, 0x04)
	asciiColor := types.MakeAttr(0x0E, 0x01)
	asciiCursor := types.MakeAttr(0x0F, 0x05)
	modColor := types.MakeAttr(0x0E, 0x04)

	totalSize := h.Source.Size()
	curRow := h.Position / BytesPerRow

	for r := 0; r < h.Size.Y; r++ {
		row := h.Top + int64(r)
		buf := screen.MakeDrawBuffer(h.Size.X)
		for x := 0; x < h.Size.X; x++ {
			screen.DrawCell(buf, x, " ", hexColor)
		}
		if row*BytesPerRow >= totalSize {
			h.WriteLine(0, r, h.Size.X, 1, buf)
			continue
		}
		// Address.
		addr := fmt.Sprintf("%08X", row*BytesPerRow)
		ac := addrColor
		if row == curRow {
			ac = types.MakeAttr(0x0F, 0x01)
		}
		for i, c := range addr {
			buf[i] = types.DrawCell{Ch: string(c), Attr: ac}
		}
		// Hex bytes + ASCII.
		for byteIdx := 0; byteIdx < BytesPerRow; byteIdx++ {
			pos := row*BytesPerRow + int64(byteIdx)
			if pos >= totalSize {
				break
			}
			b := h.Source.GetByte(pos)
			c := hexColor
			ac := asciiColor
			if row == curRow {
				c = hexFocus
			}
			if h.Source.IsByteModified(pos) {
				c = modColor
			}
			isCursor := pos == h.Position
			if isCursor && h.Mode == ModeHex {
				c = cursorColor
			}
			if isCursor && h.Mode == ModeASCII {
				ac = asciiCursor
			}
			// Hex pair.
			x := hexColStart(byteIdx)
			hi := nibbleHex(b >> 4)
			lo := nibbleHex(b & 0x0F)
			if x+1 < h.Size.X {
				buf[x] = types.DrawCell{Ch: string(hi), Attr: c}
				buf[x+1] = types.DrawCell{Ch: string(lo), Attr: c}
			}
			// ASCII.
			ax := asciiColStart() + byteIdx
			if ax < h.Size.X {
				ch := byte('.')
				if b >= 0x20 && b < 0x7F {
					ch = b
				}
				buf[ax] = types.DrawCell{Ch: string(rune(ch)), Attr: ac}
			}
		}
		h.WriteLine(0, r, h.Size.X, 1, buf)
	}
	// Cursor placement.
	cursorX, cursorY := h.cursorScreenPos()
	h.Cursor = geom.Point{X: cursorX, Y: cursorY}
}

func nibbleHex(n byte) rune {
	if n < 10 {
		return rune('0' + n)
	}
	return rune('A' + n - 10)
}

func (h *HexEditor) cursorScreenPos() (int, int) {
	row := h.Position / BytesPerRow
	col := int(h.Position % BytesPerRow)
	y := int(row - h.Top)
	if y < 0 || y >= h.Size.Y {
		return -1, -1
	}
	if h.Mode == ModeHex {
		x := hexColStart(col)
		if !h.HighNibble {
			x++
		}
		return x, y
	}
	return asciiColStart() + col, y
}

// HandleEvent implements navigation, editing, and mode toggling.
func (h *HexEditor) HandleEvent(ev *drivers.Event) {
	if h.Source == nil {
		return
	}
	if ev.What == consts.EvMouseDown {
		h.handleMouse(ev)
		return
	}
	if ev.What != consts.EvKeyDown {
		return
	}
	totalSize := h.Source.Size()
	switch ev.KeyCode {
	case consts.KbLeft:
		if h.Mode == ModeHex && !h.HighNibble {
			h.HighNibble = true
		} else {
			if h.Position > 0 {
				h.SetPosition(h.Position - 1)
				if h.Mode == ModeHex {
					h.HighNibble = false
				}
			}
		}
	case consts.KbRight:
		if h.Mode == ModeHex && h.HighNibble {
			h.HighNibble = false
		} else {
			h.SetPosition(h.Position + 1)
		}
	case consts.KbUp:
		if h.Position >= BytesPerRow {
			h.SetPosition(h.Position - BytesPerRow)
		}
	case consts.KbDown:
		if h.Position+BytesPerRow < totalSize {
			h.SetPosition(h.Position + BytesPerRow)
		}
	case consts.KbHome:
		h.SetPosition(h.Position - h.Position%BytesPerRow)
	case consts.KbEnd:
		end := h.Position - h.Position%BytesPerRow + BytesPerRow - 1
		if end >= totalSize {
			end = totalSize - 1
		}
		h.SetPosition(end)
	case consts.KbPgUp:
		h.SetPosition(h.Position - int64(h.Size.Y)*BytesPerRow)
	case consts.KbPgDn:
		h.SetPosition(h.Position + int64(h.Size.Y)*BytesPerRow)
	case consts.KbTab:
		if h.Mode == ModeHex {
			h.Mode = ModeASCII
		} else {
			h.Mode = ModeHex
			h.HighNibble = true
		}
	case consts.KbCtrlR:
		// Reload from disk (file-backed source only). Silently ignored
		// when the source isn't file-backed.
		_ = h.Source.Reload()
		if h.Position >= h.Source.Size() {
			h.Position = h.Source.Size() - 1
			if h.Position < 0 {
				h.Position = 0
			}
		}
	default:
		// Editing.
		if h.Source.CanWrite() {
			h.handleEditKey(ev)
		}
	}
	h.Draw()
	h.ClearEvent(ev)
}

// handleEditKey processes a printable / hex character at the cursor.
func (h *HexEditor) handleEditKey(ev *drivers.Event) {
	r := ev.UnicodeChar
	if r == 0 {
		return
	}
	if h.Mode == ModeASCII {
		if r >= ' ' && r < 0x7F {
			_ = h.Source.SetByte(h.Position, byte(r))
			h.SetPosition(h.Position + 1)
		}
		return
	}
	// Hex mode: only accept hex digits.
	v, ok := hexDigit(byte(r))
	if !ok {
		return
	}
	cur := h.Source.GetByte(h.Position)
	if h.HighNibble {
		cur = (cur & 0x0F) | (v << 4)
		_ = h.Source.SetByte(h.Position, cur)
		h.HighNibble = false
	} else {
		cur = (cur & 0xF0) | v
		_ = h.Source.SetByte(h.Position, cur)
		h.SetPosition(h.Position + 1)
	}
}

func hexDigit(b byte) (byte, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}

func (h *HexEditor) handleMouse(ev *drivers.Event) {
	local := h.MakeLocal(ev.Where)
	row := h.Top + int64(local.Y)
	if row < 0 {
		return
	}
	// Hex column?
	for byteIdx := 0; byteIdx < BytesPerRow; byteIdx++ {
		x := hexColStart(byteIdx)
		if local.X == x || local.X == x+1 {
			h.SetPosition(row*BytesPerRow + int64(byteIdx))
			h.Mode = ModeHex
			h.HighNibble = local.X == x
			h.Draw()
			h.ClearEvent(ev)
			return
		}
	}
	// ASCII column?
	asciiX := asciiColStart()
	if local.X >= asciiX && local.X < asciiX+BytesPerRow {
		h.SetPosition(row*BytesPerRow + int64(local.X-asciiX))
		h.Mode = ModeASCII
		h.Draw()
		h.ClearEvent(ev)
		return
	}
	if h.Owner != nil {
		h.Owner.Focus(h.Self())
	}
	h.ClearEvent(ev)
}
