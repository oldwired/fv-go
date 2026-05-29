// Command fvdemo is a Turbo-Vision-style demo program that exercises
// the fv-go widget set: menus, dialogs, list boxes, validators, and
// message boxes.
//
// It is the Go-side analogue of the Delphi FVTest.exe.
package main

import (
	"fmt"
	"os"

	"github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/drivers"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/menus"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/validators"
)

const (
	cmDialogDemo      uint16 = 100
	cmListBoxDemo     uint16 = 101
	cmValidatorDemo   uint16 = 102
	cmAboutDemo       uint16 = 103
	cmNewWindow       uint16 = 104
	cmCascadeNoResize uint16 = 110
)

func main() {
	a, err := app.NewApplication()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "fvdemo:", err)
		os.Exit(1)
	}
	defer a.Done()

	cols, rows := a.BaseView().Size.X, a.BaseView().Size.Y
	a.SetMenuBar(buildMenuBar(geom.NewRect(0, 0, cols, 1)))
	a.SetStatusLine(buildStatusLine(geom.NewRect(0, rows-1, cols, rows)))

	a.OnCommand = func(cmd uint16, ev *drivers.Event) bool {
		if dispatchWidget(a, cmd) {
			return true
		}
		if dispatchApp(a, cmd) {
			return true
		}
		if dispatchExtension(a, cmd, ev) {
			return true
		}
		switch cmd {
		case cmAboutDemo:
			showAbout(a)
			return true
		case cmDialogDemo:
			showDialogDemo(a)
			return true
		case cmListBoxDemo:
			showListBoxDemo(a)
			return true
		case cmValidatorDemo:
			showValidatorDemo(a)
			return true
		case cmNewWindow:
			openNonModalWindow(a)
			return true
		case consts.CmZoom:
			zoomFocused(a)
			return true
		case consts.CmTile:
			tileWindows(a)
			return true
		case consts.CmTileHorizontal:
			tileHorizontal(a)
			return true
		case consts.CmTileVertical:
			tileVertical(a)
			return true
		case consts.CmCascade:
			cascadeWindows(a)
			return true
		case cmCascadeNoResize:
			cascadeNoResize(a)
			return true
		case consts.CmNext:
			cycleWindow(a, +1)
			return true
		case consts.CmPrev:
			cycleWindow(a, -1)
			return true
		case consts.CmClose:
			closeFocused(a)
			return true
		case consts.CmCloseAll:
			closeAll(a)
			return true
		case consts.CmHelp:
			showHelp(a)
			return true
		case consts.CmSelectWindowNum:
			selectWindowNum(a, int(ev.InfoInt))
			return true
		}
		return false
	}

	a.Run()
}

func buildMenuBar(bounds geom.Rect) *menus.MenuBar {
	fileMenu := menus.NewMenu(
		&menus.Item{Name: "~A~bout...", Command: cmAboutDemo},
		menus.Separator(),
		&menus.Item{Name: "E~x~it", Command: consts.CmQuitApp},
	)
	appsMenu := menus.NewMenu(
		&menus.Item{Name: "~T~ext Editor", Command: cmAppEditor},
		&menus.Item{Name: "Editor (Open ~F~ile...)", Command: cmAppEditorOpen},
		&menus.Item{Name: "Editor + ~G~utter", Command: cmAppEditorWithGutter},
		&menus.Item{Name: "Editor + ~S~yntax (Go)", Command: cmAppEditorSyntax},
		menus.Separator(),
		&menus.Item{Name: "~H~ex Editor", Command: cmAppHexEdit},
		&menus.Item{Name: "Data Gri~d~", Command: cmAppGrid},
		menus.Separator(),
		&menus.Item{Name: "~L~og Viewer", Command: cmAppLogViewer},
		&menus.Item{Name: "~M~arkdown View", Command: cmAppMarkdown},
		&menus.Item{Name: "~F~uzzy Finder", Command: cmAppFuzzy},
		&menus.Item{Name: "~I~mage View", Command: cmAppImageView},
		&menus.Item{Name: "Image View (~O~pen...)", Command: cmAppImageViewOpen},
		&menus.Item{Name: "Sixel ~C~anvas", Command: cmAppCanvas},
		&menus.Item{Name: "T~e~rminal", Command: cmAppTerminal},
		&menus.Item{Name: "~G~ame of Life", Command: cmAppGameOfLife},
		menus.Separator(),
		&menus.Item{Name: "Pro~f~ile Dump", Command: cmAppProfileDump},
		&menus.Item{Name: "H~y~perlinks", Command: cmAppHyperlink},
		&menus.Item{Name: "~U~nicode Showcase", Command: cmAppUnicode},
		menus.Separator(),
		&menus.Item{Name: "Save Desktop...", Command: cmAppSaveDesktop},
		&menus.Item{Name: "Load Desktop...", Command: cmAppLoadDesktop},
	)
	systemMenu := menus.NewMenu(
		&menus.Item{Name: "~U~ptime", Command: cmAppUptime},
		&menus.Item{Name: "~C~PU Meter", Command: cmAppCPUMeter},
		&menus.Item{Name: "CPU C~o~res", Command: cmAppCPUCores},
		&menus.Item{Name: "~R~AM", Command: cmAppRAM},
		&menus.Item{Name: "~D~isk Usage", Command: cmAppDisk},
		&menus.Item{Name: "~B~attery", Command: cmAppBattery},
		&menus.Item{Name: "~N~etwork", Command: cmAppNetwork},
		&menus.Item{Name: "~P~rocesses", Command: cmAppProcess},
	)
	testMenu := menus.NewMenu(
		&menus.Item{Name: "~N~ew Window", Command: cmNewWindow},
		menus.Separator(),
		&menus.Item{Name: "~A~pps", Sub: appsMenu},
		&menus.Item{Name: "~S~ystem", Sub: systemMenu},
		menus.Separator(),
		&menus.Item{Name: "~D~ialog with all controls", Command: cmDialogDemo},
		&menus.Item{Name: "~L~ist Box", Command: cmListBoxDemo},
		&menus.Item{Name: "~V~alidators", Command: cmValidatorDemo},
	)
	windowMenu := menus.NewMenu(
		&menus.Item{Name: "~Z~oom", Command: consts.CmZoom},
		menus.Separator(),
		&menus.Item{Name: "~T~ile (grid)", Command: consts.CmTile},
		&menus.Item{Name: "Tile ~H~orizontal", Command: consts.CmTileHorizontal},
		&menus.Item{Name: "Tile ~V~ertical", Command: consts.CmTileVertical},
		menus.Separator(),
		&menus.Item{Name: "~C~ascade", Command: consts.CmCascade},
		&menus.Item{Name: "Cascade (Keep ~S~izes)", Command: cmCascadeNoResize},
		menus.Separator(),
		&menus.Item{Name: "~N~ext", Command: consts.CmNext},
		&menus.Item{Name: "~P~revious", Command: consts.CmPrev},
		menus.Separator(),
		&menus.Item{Name: "C~l~ose", Command: consts.CmClose},
		&menus.Item{Name: "Close ~A~ll", Command: consts.CmCloseAll},
	)
	return menus.NewMenuBar(bounds, menus.NewMenu(
		&menus.Item{Name: "~F~ile", Sub: fileMenu},
		&menus.Item{Name: "~T~est", Sub: testMenu},
		&menus.Item{Name: "Widg~e~ts", Sub: buildWidgetsMenu()},
		&menus.Item{Name: "~W~indow", Sub: windowMenu},
	))
}

func buildWidgetsMenu() *menus.Menu {
	indicators := menus.NewMenu(
		&menus.Item{Name: "~P~rogressBar", Command: cmWidgetProgressBar},
		&menus.Item{Name: "~S~pinner", Command: cmWidgetSpinner},
		&menus.Item{Name: "~T~ask Progress", Command: cmWidgetTaskProgress},
		&menus.Item{Name: "~V~U Meter", Command: cmWidgetVUMeter},
		&menus.Item{Name: "Spar~k~line", Command: cmWidgetSparkline},
		&menus.Item{Name: "~B~ar Chart", Command: cmWidgetBarChart},
		&menus.Item{Name: "~L~ED Digits", Command: cmWidgetLEDDigits},
		&menus.Item{Name: "~M~arquee", Command: cmWidgetMarquee},
		&menus.Item{Name: "Bl~i~nk", Command: cmWidgetBlink},
		&menus.Item{Name: "B~r~eadcrumb", Command: cmWidgetBreadcrumb},
		menus.Separator(),
		&menus.Item{Name: "~D~igital Clock", Command: cmWidgetClockDigital},
		&menus.Item{Name: "A~n~alog Clock", Command: cmWidgetClockAnalog},
		&menus.Item{Name: "Heap Vi~e~w", Command: cmWidgetHeapView},
	)
	inputs := menus.NewMenu(
		&menus.Item{Name: "~C~ombo Box", Command: cmWidgetComboBox},
		&menus.Item{Name: "C~h~eck List", Command: cmWidgetCheckList},
		&menus.Item{Name: "~T~oggle Switch", Command: cmWidgetToggle},
		&menus.Item{Name: "Input ~L~ong", Command: cmWidgetInputLong},
		&menus.Item{Name: "Tree ~V~iew", Command: cmWidgetTreeView},
		&menus.Item{Name: "C~a~lendar", Command: cmWidgetCalendar},
		&menus.Item{Name: "Color ~S~elector", Command: cmWidgetColorSel},
		&menus.Item{Name: "~H~istory Dropdown", Command: cmWidgetHistory},
	)
	layout := menus.NewMenu(
		&menus.Item{Name: "Ta~b~s", Command: cmWidgetTabs},
		&menus.Item{Name: "~A~ccordion", Command: cmWidgetAccordion},
		&menus.Item{Name: "~T~oolBar", Command: cmWidgetToolBar},
		&menus.Item{Name: "~C~olored Text", Command: cmWidgetColorTxt},
		&menus.Item{Name: "AS~C~II Chart", Command: cmWidgetAsciiTab},
	)
	popups := menus.NewMenu(
		&menus.Item{Name: "~N~otification", Command: cmWidgetNotification},
		&menus.Item{Name: "Tool~t~ip", Command: cmWidgetTooltip},
		&menus.Item{Name: "~P~opup Menu", Command: cmWidgetPopupMenu},
		&menus.Item{Name: "Timed ~D~ialog", Command: cmWidgetTimedDlg},
		menus.Separator(),
		&menus.Item{Name: "File ~O~pen...", Command: cmWidgetFileOpen},
		&menus.Item{Name: "File ~S~ave...", Command: cmWidgetFileSave},
		&menus.Item{Name: "Change Di~r~...", Command: cmWidgetChangeDir},
	)
	return menus.NewMenu(
		&menus.Item{Name: "~I~ndicators", Sub: indicators},
		&menus.Item{Name: "I~n~puts", Sub: inputs},
		&menus.Item{Name: "~L~ayout", Sub: layout},
		&menus.Item{Name: "~P~opups", Sub: popups},
	)
}

func buildStatusLine(bounds geom.Rect) *menus.StatusLine {
	// The Program-level shortcuts (F1/F5/F6/Alt-F3/Alt-0..9) are always
	// active regardless of the status line; the items here just hint at
	// the most useful ones plus the primary entry points.
	return menus.NewStatusLine(bounds, []*menus.StatusItem{
		{Text: "~F1~ Help", KeyCode: consts.KbF1, Command: consts.CmHelp},
		{Text: "~Alt-X~ Exit", KeyCode: consts.KbAltX, Command: consts.CmQuitApp},
		{Text: "~F10~ Menu", KeyCode: consts.KbF10, Command: consts.CmMenu},
		{Text: "~F6~ Next", KeyCode: 0, Command: 0},
		{Text: "~Alt-F3~ Close", KeyCode: 0, Command: 0},
	})
}

func showAbout(a *app.Application) {
	msgbox.Show(&a.Desktop.Group,
		msgbox.Info,
		"fv-go demo\n\nA Go port of Free Vision.\n\nPress OK to dismiss.",
		msgbox.OKOnly)
}

func showDialogDemo(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 50, 14), "All Controls")

	// Label + InputLine
	il := dialogs.NewInputLine(geom.NewRect(15, 2, 45, 3), 64)
	il.SetText("type here")
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 2, 14, 3), "~N~ame:", il))
	d.Insert(il)

	// CheckBoxes
	checks := dialogs.NewCheckBoxes(geom.NewRect(2, 4, 22, 7), []string{
		"~B~old", "~I~talic", "~U~nderline",
	})
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 3, 22, 4), "Style", nil))
	d.Insert(checks)

	// RadioButtons
	radios := dialogs.NewRadioButtons(geom.NewRect(24, 4, 45, 7), []string{
		"~L~eft", "~C~enter", "~R~ight",
	})
	d.Insert(dialogs.NewLabel(geom.NewRect(24, 3, 44, 4), "Align", nil))
	d.Insert(radios)

	// Buttons
	d.Insert(dialogs.NewButton(geom.NewRect(13, 11, 23, 12), "O~K~", consts.CmOK, dialogs.BfDefault))
	d.Insert(dialogs.NewButton(geom.NewRect(28, 11, 38, 12), "Cancel", consts.CmCancel, 0))

	cmd := a.Desktop.ExecView(d)
	if cmd == consts.CmOK {
		msgbox.Showf(&a.Desktop.Group, msgbox.Info,
			"Got: name=%q checks=%d radios=%d",
			[]any{il.Text(), checks.Value, radios.Value},
			msgbox.OKOnly)
	}
}

func showListBoxDemo(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 40, 14), "Pick One")
	scroll := views_NewScrollBar(geom.NewRect(38, 1, 39, 12))
	d.Insert(scroll)
	list := dialogs.NewStringListBox(geom.NewRect(2, 1, 38, 12), scroll, []string{
		"Apple", "Banana", "Cherry", "Date", "Elderberry", "Fig", "Grape",
		"Honeydew", "Kiwi", "Lemon", "Mango", "Nectarine", "Orange",
	})
	d.Insert(list)
	d.Insert(dialogs.NewButton(geom.NewRect(8, 12, 18, 13), "O~K~", consts.CmOK, dialogs.BfDefault))
	d.Insert(dialogs.NewButton(geom.NewRect(22, 12, 32, 13), "Cancel", consts.CmCancel, 0))

	if a.Desktop.ExecView(d) == consts.CmOK {
		msgbox.Showf(&a.Desktop.Group, msgbox.Info,
			"You picked: %s", []any{list.Items[list.Focused]}, msgbox.OKOnly)
	}
}

func showValidatorDemo(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 50, 12), "Validators")

	il := dialogs.NewInputLine(geom.NewRect(20, 2, 47, 3), 5)
	il.Validator = validators.NewPXPictureValidator("###-##", true)
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 2, 19, 3), "~Z~ip (NNN-NN):", il))
	d.Insert(il)

	il2 := dialogs.NewInputLine(geom.NewRect(20, 4, 30, 5), 4)
	il2.Validator = validators.NewRangeValidator(1, 999)
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 4, 19, 5), "~A~ge (1-999):", il2))
	d.Insert(il2)

	d.Insert(dialogs.NewButton(geom.NewRect(13, 9, 23, 10), "O~K~", consts.CmOK, dialogs.BfDefault))
	d.Insert(dialogs.NewButton(geom.NewRect(28, 9, 38, 10), "Cancel", consts.CmCancel, 0))

	a.Desktop.ExecView(d)
}
