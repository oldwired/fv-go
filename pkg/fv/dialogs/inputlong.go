package dialogs

import (
	"strconv"

	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/validators"
)

// InputLong is an InputLine that holds a 64-bit integer in [Min, Max].
// Internally it's a regular InputLine guarded by a RangeValidator,
// with helpers to round-trip the int value.
type InputLong struct {
	InputLine
	Min, Max int64
}

// NewInputLong builds an InputLong with the given numeric range.
func NewInputLong(bounds geom.Rect, min, max int64, maxLen int) *InputLong {
	il := &InputLong{
		InputLine: *NewInputLine(bounds, maxLen),
		Min:       min,
		Max:       max,
	}
	il.SetSelf(il)
	il.Validator = validators.NewRangeValidator(min, max)
	return il
}

// GetTypeID for serial registry.
func (il *InputLong) GetTypeID() string { return "inputlong" }

// SetInt installs v as the displayed value, clamping to [Min, Max].
func (il *InputLong) SetInt(v int64) {
	if v < il.Min {
		v = il.Min
	}
	if v > il.Max {
		v = il.Max
	}
	il.SetText(strconv.FormatInt(v, 10))
}

// Int returns the parsed numeric value, or 0 if the field is empty
// or unparseable.
func (il *InputLong) Int() int64 {
	v, err := strconv.ParseInt(il.Text(), 10, 64)
	if err != nil {
		return 0
	}
	return v
}

// Valid uses the underlying RangeValidator unless the caller is
// cancelling.
func (il *InputLong) Valid(cmd uint16) bool {
	if cmd == consts.CmCancel {
		return true
	}
	return il.InputLine.Valid(cmd)
}
