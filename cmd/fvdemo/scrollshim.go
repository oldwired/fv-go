package main

import (
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/views"
)

// views_NewScrollBar is a thin shim that the linter prefers over an
// alias declaration in the same file.
func views_NewScrollBar(r geom.Rect) *views.ScrollBar {
	return views.NewScrollBar(r)
}
