package main

import (
	"runtime"
	"time"

	"github.com/oldwired/fv-go/pkg/fv/app"
	"github.com/oldwired/fv-go/pkg/fv/consts"
	"github.com/oldwired/fv-go/pkg/fv/dialogs"
	"github.com/oldwired/fv-go/pkg/fv/geom"
	"github.com/oldwired/fv-go/pkg/fv/history"
	"github.com/oldwired/fv-go/pkg/fv/msgbox"
	"github.com/oldwired/fv-go/pkg/fv/types"
	"github.com/oldwired/fv-go/pkg/fv/views"
	"github.com/oldwired/fv-go/pkg/fv/widgets/accordion"
	"github.com/oldwired/fv-go/pkg/fv/widgets/asciitab"
	"github.com/oldwired/fv-go/pkg/fv/widgets/barchart"
	"github.com/oldwired/fv-go/pkg/fv/widgets/blink"
	"github.com/oldwired/fv-go/pkg/fv/widgets/breadcrumb"
	"github.com/oldwired/fv-go/pkg/fv/widgets/calendar"
	"github.com/oldwired/fv-go/pkg/fv/widgets/clock"
	"github.com/oldwired/fv-go/pkg/fv/widgets/colorsel"
	"github.com/oldwired/fv-go/pkg/fv/widgets/colortxt"
	"github.com/oldwired/fv-go/pkg/fv/widgets/combobox"
	"github.com/oldwired/fv-go/pkg/fv/widgets/heapview"
	"github.com/oldwired/fv-go/pkg/fv/widgets/leddigits"
	"github.com/oldwired/fv-go/pkg/fv/widgets/marquee"
	"github.com/oldwired/fv-go/pkg/fv/widgets/notification"
	"github.com/oldwired/fv-go/pkg/fv/widgets/popupmenu"
	"github.com/oldwired/fv-go/pkg/fv/widgets/progressbar"
	"github.com/oldwired/fv-go/pkg/fv/widgets/sparkline"
	"github.com/oldwired/fv-go/pkg/fv/widgets/spinner"
	"github.com/oldwired/fv-go/pkg/fv/widgets/stddlg"
	"github.com/oldwired/fv-go/pkg/fv/widgets/tabs"
	"github.com/oldwired/fv-go/pkg/fv/widgets/taskprogress"
	"github.com/oldwired/fv-go/pkg/fv/widgets/timeddlg"
	"github.com/oldwired/fv-go/pkg/fv/widgets/toggle"
	"github.com/oldwired/fv-go/pkg/fv/widgets/toolbar"
	"github.com/oldwired/fv-go/pkg/fv/widgets/tooltip"
	"github.com/oldwired/fv-go/pkg/fv/widgets/treeview"
	"github.com/oldwired/fv-go/pkg/fv/widgets/vumeter"
)

// Demo command IDs for individual widget showcases (above 200 to avoid
// collisions with the framework's CmXxx range and our other demo IDs).
const (
	cmWidgetProgressBar uint16 = 200 + iota
	cmWidgetSpinner
	cmWidgetTaskProgress
	cmWidgetVUMeter
	cmWidgetSparkline
	cmWidgetBarChart
	cmWidgetLEDDigits
	cmWidgetMarquee
	cmWidgetBlink
	cmWidgetBreadcrumb
	cmWidgetComboBox
	cmWidgetCheckList
	cmWidgetToggle
	cmWidgetInputLong
	cmWidgetTreeView
	cmWidgetCalendar
	cmWidgetColorSel
	cmWidgetTabs
	cmWidgetAccordion
	cmWidgetToolBar
	cmWidgetColorTxt
	cmWidgetAsciiTab
	cmWidgetNotification
	cmWidgetTooltip
	cmWidgetPopupMenu
	cmWidgetTimedDlg
	cmWidgetFileOpen
	cmWidgetFileSave
	cmWidgetChangeDir
	cmWidgetClockDigital
	cmWidgetClockAnalog
	cmWidgetHeapView
	cmWidgetHistory
)

// dispatchWidget routes widget-menu commands. Returns true when the
// command was a known widget showcase. Wired from main's OnCommand
// before falling through to other handlers.
func dispatchWidget(a *app.Application, cmd uint16) bool {
	switch cmd {
	case cmWidgetProgressBar:
		showProgressBar(a)
	case cmWidgetSpinner:
		showSpinner(a)
	case cmWidgetTaskProgress:
		showTaskProgress(a)
	case cmWidgetVUMeter:
		showVUMeter(a)
	case cmWidgetSparkline:
		showSparkline(a)
	case cmWidgetBarChart:
		showBarChart(a)
	case cmWidgetLEDDigits:
		showLEDDigits(a)
	case cmWidgetMarquee:
		showMarquee(a)
	case cmWidgetBlink:
		showBlink(a)
	case cmWidgetBreadcrumb:
		showBreadcrumb(a)
	case cmWidgetComboBox:
		showComboBox(a)
	case cmWidgetCheckList:
		showCheckList(a)
	case cmWidgetToggle:
		showToggle(a)
	case cmWidgetInputLong:
		showInputLong(a)
	case cmWidgetTreeView:
		showTreeView(a)
	case cmWidgetCalendar:
		showCalendar(a)
	case cmWidgetColorSel:
		showColorSel(a)
	case cmWidgetTabs:
		showTabs(a)
	case cmWidgetAccordion:
		showAccordion(a)
	case cmWidgetToolBar:
		showToolBar(a)
	case cmWidgetColorTxt:
		showColorTxt(a)
	case cmWidgetAsciiTab:
		showAsciiTab(a)
	case cmWidgetNotification:
		showNotification(a)
	case cmWidgetTooltip:
		showTooltip(a)
	case cmWidgetPopupMenu:
		showPopupMenu(a)
	case cmWidgetTimedDlg:
		showTimedDlg(a)
	case cmWidgetFileOpen:
		showFileOpen(a)
	case cmWidgetFileSave:
		showFileSave(a)
	case cmWidgetChangeDir:
		showChangeDir(a)
	case cmWidgetClockDigital:
		showClockDigital(a)
	case cmWidgetClockAnalog:
		showClockAnalog(a)
	case cmWidgetHeapView:
		showHeapView(a)
	case cmWidgetHistory:
		showHistory(a)
	default:
		return false
	}
	return true
}

// --- showcase wrappers ------------------------------------------------

func showProgressBar(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 50, 8), "Progress Bar")
	bar := progressbar.New(geom.NewRect(2, 2, 48, 4), 0, 100)
	bar.Position = 42
	d.Insert(bar)
	d.Insert(dialogs.NewButton(geom.NewRect(20, 5, 30, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showSpinner(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 30, 8), "Spinner")
	sp := spinner.New(geom.NewRect(2, 2, 28, 4))
	sp.Set = spinner.Pulse
	sp.Start()
	d.Insert(sp)
	d.Insert(dialogs.NewButton(geom.NewRect(10, 5, 20, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
	sp.Stop()
}

func showTaskProgress(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 64, 12), "Task Progress")
	tp := taskprogress.New(geom.NewRect(2, 2, 62, 9))
	t1 := tp.AddTask("Indexing", 0, 100)
	t1.Value = 73
	t2 := tp.AddTask("Compiling", 0, 100)
	t2.Value = 28
	t3 := tp.AddTask("Linking", 0, 100)
	t3.Done = true
	t3.Value = 100
	d.Insert(tp)
	d.Insert(dialogs.NewButton(geom.NewRect(27, 9, 37, 10), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showVUMeter(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 50, 8), "VU Meter")
	vu := vumeter.New(geom.NewRect(2, 2, 48, 4))
	vu.SetLevel(0.7)
	d.Insert(vu)
	d.Insert(dialogs.NewButton(geom.NewRect(20, 5, 30, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showSparkline(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 60, 8), "Sparkline")
	sp := sparkline.New(geom.NewRect(2, 2, 58, 4), 56)
	for i := 0; i < 56; i++ {
		sp.Push(float64(((i*37)%100)+10) + 0.0)
	}
	d.Insert(sp)
	d.Insert(dialogs.NewButton(geom.NewRect(25, 5, 35, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showBarChart(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 50, 12), "Bar Chart")
	chart := barchart.New(geom.NewRect(2, 2, 48, 9))
	chart.SetBars([]barchart.Bar{
		{Label: "Apples", Value: 23},
		{Label: "Bananas", Value: 41},
		{Label: "Cherries", Value: 12},
		{Label: "Dragon", Value: 35},
		{Label: "Elderberry", Value: 8},
	})
	d.Insert(chart)
	d.Insert(dialogs.NewButton(geom.NewRect(20, 9, 30, 10), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showLEDDigits(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 36, 8), "LED Digits")
	led := leddigits.New(geom.NewRect(2, 2, 34, 5), 6)
	led.SetValue(123456)
	d.Insert(led)
	d.Insert(dialogs.NewButton(geom.NewRect(13, 5, 23, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showMarquee(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 50, 8), "Marquee")
	m := marquee.New(geom.NewRect(2, 2, 48, 3), "Welcome to fv-go widgets — keep watching the text crawl by!")
	d.Insert(m)
	d.Insert(dialogs.NewButton(geom.NewRect(20, 5, 30, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showBlink(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 30, 8), "Blink Indicator")
	b := blink.New(geom.NewRect(2, 2, 4, 4))
	b.SetMode(blink.StateBlinking)
	d.Insert(b)
	d.Insert(dialogs.NewStaticText(geom.NewRect(7, 2, 28, 3), "Activity..."))
	d.Insert(dialogs.NewButton(geom.NewRect(10, 5, 20, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
	b.SetMode(blink.StateOff)
}

func showBreadcrumb(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 60, 8), "Breadcrumb")
	bc := breadcrumb.New(geom.NewRect(2, 2, 58, 3), []breadcrumb.Segment{
		{Label: "Home"}, {Label: "Projects"}, {Label: "fv-go"}, {Label: "widgets"},
	})
	d.Insert(bc)
	d.Insert(dialogs.NewButton(geom.NewRect(25, 5, 35, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showComboBox(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 40, 8), "Combo Box")
	cb := combobox.New(geom.NewRect(2, 2, 38, 3), []string{
		"Apple", "Banana", "Cherry", "Date", "Elderberry",
	}, 32)
	d.Insert(cb)
	d.Insert(dialogs.NewButton(geom.NewRect(15, 5, 25, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showCheckList(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 40, 12), "Check List")
	scroll := views.NewScrollBar(geom.NewRect(38, 2, 39, 9))
	d.Insert(scroll)
	cl := dialogs.NewCheckListBox(geom.NewRect(2, 2, 38, 9), scroll, []string{
		"Bold", "Italic", "Underline", "Strikethrough", "Overline", "Dim",
	})
	d.Insert(cl)
	d.Insert(dialogs.NewButton(geom.NewRect(15, 9, 25, 10), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showToggle(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 40, 10), "Toggle")
	t1 := toggle.New(geom.NewRect(2, 2, 38, 3), "~B~old", 0, false)
	t1.Style = toggle.StyleSlider
	t2 := toggle.New(geom.NewRect(2, 4, 38, 5), "~I~talic", 0, true)
	t2.Style = toggle.StyleCheckbox
	t3 := toggle.New(geom.NewRect(2, 6, 38, 7), "~D~ebug", 0, false)
	t3.Style = toggle.StyleBrackets
	d.Insert(t1)
	d.Insert(t2)
	d.Insert(t3)
	d.Insert(dialogs.NewButton(geom.NewRect(15, 8, 25, 9), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showInputLong(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 36, 8), "Input Long")
	il := dialogs.NewInputLong(geom.NewRect(20, 2, 32, 3), 1, 9999, 4)
	il.SetInt(1234)
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 2, 19, 3), "~A~ge (1-9999):", &il.InputLine))
	d.Insert(il)
	d.Insert(dialogs.NewButton(geom.NewRect(13, 5, 23, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showTreeView(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 40, 14), "Tree View")
	root := &treeview.Node{Label: "fv-go", Expanded: true, Children: []*treeview.Node{
		{Label: "pkg", Expanded: true, Children: []*treeview.Node{
			{Label: "fv", Children: []*treeview.Node{
				{Label: "views"}, {Label: "dialogs"}, {Label: "widgets"},
			}},
		}},
		{Label: "cmd", Children: []*treeview.Node{
			{Label: "fvdemo"},
		}},
	}}
	tree := treeview.New(geom.NewRect(2, 2, 38, 11), []*treeview.Node{root})
	d.Insert(tree)
	d.Insert(dialogs.NewButton(geom.NewRect(15, 11, 25, 12), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showCalendar(a *app.Application) {
	if d, ok := calendar.ShowDialog(&a.Desktop.Group, time.Now(), "Pick a date"); ok {
		msgbox.Showf(&a.Desktop.Group, msgbox.Info,
			"You picked: %s", []any{d.Format("Mon 2006-01-02")}, msgbox.OKOnly)
	}
}

func showColorSel(a *app.Application) {
	if c, ok := colorsel.ShowDialog(&a.Desktop.Group, 1); ok {
		msgbox.Showf(&a.Desktop.Group, msgbox.Info,
			"Color #%d (%s)", []any{c, colorsel.ColorNames[c]}, msgbox.OKOnly)
	}
}

func showAccordion(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 50, 18), "Accordion")
	acc := accordion.New(geom.NewRect(2, 2, 48, 15))
	acc.AddSection("General", dialogs.NewStaticText(geom.NewRect(0, 0, 0, 0), "General settings here"), 3)
	acc.AddSection("Appearance", dialogs.NewStaticText(geom.NewRect(0, 0, 0, 0), "Appearance settings here"), 3)
	acc.AddSection("Advanced", dialogs.NewStaticText(geom.NewRect(0, 0, 0, 0), "Advanced settings here"), 3)
	d.Insert(acc)
	d.Insert(dialogs.NewButton(geom.NewRect(20, 15, 30, 16), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showToolBar(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 60, 8), "Tool Bar")
	tb := toolbar.New(geom.NewRect(2, 2, 58, 3), []toolbar.Item{
		{Text: "~N~ew", Command: 100},
		{Text: "~O~pen", Command: 101},
		toolbar.Separator(),
		{Text: "~S~ave", Command: 102},
	})
	d.Insert(tb)
	d.Insert(dialogs.NewButton(geom.NewRect(25, 5, 35, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showColorTxt(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 50, 8), "Colored Text")
	yellowOnBlack := types.MakeAttr(0x0E, 0x00)
	d.Insert(colortxt.New(geom.NewRect(2, 2, 48, 3), "  Bright-yellow text on black!", yellowOnBlack))
	d.Insert(dialogs.NewButton(geom.NewRect(20, 5, 30, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

func showAsciiTab(a *app.Application) {
	if cp := asciitab.Show(&a.Desktop.Group); cp >= 0 {
		msgbox.Showf(&a.Desktop.Group, msgbox.Info,
			"Picked %d (%c)", []any{cp, rune(cp)}, msgbox.OKOnly)
	}
}

func showNotification(a *app.Application) {
	n := notification.New(&a.Desktop.Group, "Hello",
		"This toast will dismiss\nin 3 seconds.", notification.PosTopRight, 30, 3*time.Second)
	a.Desktop.Insert(n)
}

func showPopupMenu(a *app.Application) {
	pop := popupmenu.New(geom.Point{X: 10, Y: 5},
		[]string{"Cut", "Copy", "Paste", "Clear", "Select All"}, 30)
	idx := pop.Run(&a.Desktop.Group)
	if idx >= 0 {
		msgbox.Showf(&a.Desktop.Group, msgbox.Info,
			"Picked: %s", []any{pop.Items[idx]}, msgbox.OKOnly)
	}
}

func showTimedDlg(a *app.Application) {
	d := timeddlg.New(geom.NewRect(0, 0, 40, 8), "Auto-close demo", 5*time.Second)
	tx := timeddlg.NewText(geom.NewRect(2, 2, 38, 3), "This dialog auto-closes in %ds.")
	d.AttachCountdown(tx)
	d.Insert(tx)
	d.Insert(dialogs.NewButton(geom.NewRect(15, 5, 25, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(&d.Dialog)
}

func showFileOpen(a *app.Application) {
	if path, ok := stddlg.ShowModern(&a.Desktop.Group, stddlg.ModeOpen, "Open File", "", "*"); ok {
		msgbox.Showf(&a.Desktop.Group, msgbox.Info, "Opened: %s", []any{path}, msgbox.OKOnly)
	}
}

func showFileSave(a *app.Application) {
	if path, ok := stddlg.ShowModern(&a.Desktop.Group, stddlg.ModeSave, "Save File", "", "*"); ok {
		msgbox.Showf(&a.Desktop.Group, msgbox.Info, "Save to: %s", []any{path}, msgbox.OKOnly)
	}
}

func showChangeDir(a *app.Application) {
	if path, ok := stddlg.ShowModern(&a.Desktop.Group, stddlg.ModeChangeDir, "Change Directory", "", "*"); ok {
		msgbox.Showf(&a.Desktop.Group, msgbox.Info, "Now in: %s", []any{path}, msgbox.OKOnly)
	}
}

// Tab demo command IDs (just used inside this dialog).
const (
	cmDemoNewTab   uint16 = 220
	cmDemoCloseTab uint16 = 221
)

// showTabs builds a tab dialog seeded with three tabs and exposes
// New / Close buttons that exercise dynamic add / delete. Uses a
// custom modal loop because ExecView only terminates on the
// standard cmOK/cmCancel/cmYes/cmNo set, so cmDemoNewTab/Close
// would otherwise be silently swallowed.
func showTabs(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 60, 18), "Tabs")
	tw := tabs.New(geom.NewRect(2, 2, 58, 13))

	addPage := func(title, body string) {
		tw.AddTab(title, dialogs.NewStaticText(geom.NewRect(0, 0, 0, 0), body))
	}
	addPage("~G~reeting", "Welcome to tab #1.\n\nClick another tab title or press\nCtrl+Tab to cycle through.")
	addPage("~O~ptions", "Tab #2 — pretend there are\nradio buttons here.")
	addPage("~D~ata", "Tab #3 — pretend there's\na bar chart here.")

	d.Insert(tw)
	d.Insert(dialogs.NewButton(geom.NewRect(2, 14, 14, 15), "~N~ew Tab", cmDemoNewTab, 0))
	d.Insert(dialogs.NewButton(geom.NewRect(16, 14, 28, 15), "~C~lose Tab", cmDemoCloseTab, 0))
	d.Insert(dialogs.NewButton(geom.NewRect(46, 14, 56, 15), "~D~one", consts.CmCancel, dialogs.BfDefault))

	host := &a.Desktop.Group
	host.Insert(d)
	defer host.Delete(d)
	host.Focus(d)
	q := views.GetEventQueue()
	if q == nil {
		return
	}
	tabCounter := 4
	for {
		if pump := views.GetPump(); pump != nil {
			pump()
		}
		ev, ok := q.Get()
		if !ok {
			if wait := views.GetWait(); wait != nil {
				wait()
			}
			continue
		}
		if ev.What == consts.EvCommand {
			switch ev.Command {
			case cmDemoNewTab:
				title := "Tab " + itoa(tabCounter)
				body := "Dynamically created tab #" + itoa(tabCounter) + "."
				tw.AddTab(title, dialogs.NewStaticText(geom.NewRect(0, 0, 0, 0), body))
				tw.SetCurrent(len(tw.Items) - 1)
				tabCounter++
				continue
			case cmDemoCloseTab:
				if len(tw.Items) > 0 {
					tw.DeleteTab(tw.CurrentIndex())
				}
				continue
			case consts.CmOK, consts.CmCancel:
				return
			}
		}
		d.HandleEvent(&ev)
		views.MarkDirty()
	}
}

// itoa is a tiny base-10 formatter so the demo doesn't pull strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [12]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// tippedInput wraps an InputLine with a static TipText so the Tooltip
// widget has something to show when this field is focused.
type tippedInput struct {
	dialogs.InputLine
	Tip string
}

func (t *tippedInput) TipText() string { return t.Tip }

func newTippedInput(bounds geom.Rect, tip string) *tippedInput {
	t := &tippedInput{InputLine: *dialogs.NewInputLine(bounds, 64), Tip: tip}
	t.SetSelf(t)
	return t
}

// showTooltip puts two tipped input lines inside a dialog and starts
// a Tooltip pinned to the desktop. Hover focus on either field for
// ~700ms to see the tip pop up.
func showTooltip(a *app.Application) {
	tip := tooltip.New(&a.Desktop.Group)
	a.Desktop.Insert(tip)
	defer a.Desktop.Delete(tip.Self())

	d := dialogs.NewDialog(geom.NewRect(0, 0, 50, 10), "Tooltip Demo")
	name := newTippedInput(geom.NewRect(15, 2, 45, 3),
		"Your full name. Tab away to dismiss the tip.")
	email := newTippedInput(geom.NewRect(15, 4, 45, 5),
		"A working email address. We'll never share it.")
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 2, 14, 3), "~N~ame:", &name.InputLine))
	d.Insert(name)
	d.Insert(dialogs.NewLabel(geom.NewRect(2, 4, 14, 5), "~E~mail:", &email.InputLine))
	d.Insert(email)
	d.Insert(dialogs.NewButton(geom.NewRect(20, 7, 30, 8), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

// showClockDigital opens a digital clock face with the date on top.
// Tab through the dialog: focus moves around the buttons but the clock
// keeps ticking — its anim.Register call drives MarkDirty on every
// second regardless of focus.
func showClockDigital(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 36, 9), "Digital Clock")
	c := clock.NewDigital(geom.NewRect(2, 2, 34, 6))
	c.ShowDate = true
	c.BlinkColon = true
	d.Insert(c)
	d.Insert(dialogs.NewButton(geom.NewRect(13, 6, 23, 7), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

// showClockAnalog opens the analog face with a smooth-sweep second
// hand so the redraw rate is visibly higher than the digital cousin.
// Bounds are roughly 2:1 so the aspect-ratio correction renders a
// near-circular face.
func showClockAnalog(a *app.Application) {
	d := dialogs.NewDialog(geom.NewRect(0, 0, 36, 18), "Analog Clock")
	c := clock.NewAnalog(geom.NewRect(2, 2, 34, 15))
	c.SetSmoothSweep(true)
	c.Numerals = clock.NumeralsAll
	d.Insert(c)
	d.Insert(dialogs.NewButton(geom.NewRect(13, 15, 23, 16), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}

// showHeapView surfaces the runtime's HeapAlloc + GC count. Allocate
// a sizable slice on entry so the number is interesting rather than
// "0 B" — the slice is intentionally kept alive until the dialog
// closes (no `_ =` capture) so the GC doesn't reclaim it mid-demo.
func showHeapView(a *app.Application) {
	junk := make([]byte, 4<<20) // ~4 MiB to lift the number off zero
	d := dialogs.NewDialog(geom.NewRect(0, 0, 40, 8), "Heap View")
	hv := heapview.New(geom.NewRect(2, 2, 38, 4))
	hv.ShowGC = true
	d.Insert(hv)
	d.Insert(dialogs.NewButton(geom.NewRect(15, 5, 25, 6), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
	runtime.KeepAlive(junk)
}

// showHistory demonstrates the History dropdown wired to a real
// InputLine. We pre-seed the history store so there's something in
// the popup; the user can also Up/Down through entries directly.
func showHistory(a *app.Application) {
	const id byte = 13
	history.Clear(id)
	history.Add(id, "alpha")
	history.Add(id, "beta")
	history.Add(id, "gamma")
	history.Add(id, "the quick brown fox")

	d := dialogs.NewDialog(geom.NewRect(0, 0, 56, 10), "History Popup")
	d.Insert(dialogs.NewStaticText(geom.NewRect(2, 2, 54, 4),
		"Click the ▾ to pick a past entry, or press ↑/↓\nfrom inside the input to scroll through them."))
	il := dialogs.NewInputLine(geom.NewRect(2, 5, 50, 6), 64)
	il.HistoryID = id
	d.Insert(il)
	d.Insert(dialogs.NewHistory(geom.NewRect(50, 5, 54, 6), il, int(id)))
	d.Insert(dialogs.NewButton(geom.NewRect(23, 7, 33, 8), "O~K~", consts.CmOK, dialogs.BfDefault))
	a.Desktop.ExecView(d)
}
