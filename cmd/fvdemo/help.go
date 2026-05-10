package main

import (
	"github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
)

// showHelp displays a static help dialog. In a real app you'd hook
// this up to per-view HelpCtx values and a help-text resource — the
// stub is a placeholder so F1 has *something* to show.
func showHelp(a *app.Application) {
	msgbox.Show(&a.Desktop.Group, msgbox.Info,
		"fv-go demo — keyboard reference\n\n"+
			"F1            this help\n"+
			"F5            zoom (maximize / restore) the active window\n"+
			"F6 / Sh+F6    next / previous window\n"+
			"Alt-F3        close active window\n"+
			"Alt-0..9      select window by number\n"+
			"F10           open menu bar\n"+
			"Alt-X         quit\n"+
			"Tab / Sh+Tab  cycle focus inside a dialog\n"+
			"Ctrl+A        select all in an input line\n"+
			"Ctrl+C/X/V    copy / cut / paste",
		msgbox.OKOnly)
}
