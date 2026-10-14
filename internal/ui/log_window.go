package ui

import (
	"fmt"
	"time"

	"jandy.cz/ess-provisioner/internal/ui/win32"
)

// Size of the inside of the log window when it first opens, in layout units.
const (
	logWindowWidth  = 760
	logWindowHeight = 460
)

// logWindow is a resizable window with the progress messages of the program.
// It is created hidden at start-up so no message is lost, and closing it only
// hides it again.
type logWindow struct {
	app  *app
	hwnd win32.HWND
	text control
}

func newLogWindow(a *app) (*logWindow, error) {
	w := &logWindow{app: a}
	hwnd, err := a.createTopLevelWindow(win32.TopLevelWindow{
		ClassName:    logWindowClass,
		Title:        a.cfg.ProductName + " — Log",
		Style:        win32.WS_RESIZABLEWINDOW,
		ClientWidth:  logWindowWidth,
		ClientHeight: logWindowHeight,
		Owner:        a.main.hwnd,
	}, w.handleMessage)
	if err != nil {
		return nil, err
	}
	w.hwnd = hwnd

	f := a.newForm(hwnd)
	w.text = f.logView("", clientArea(logWindowWidth, logWindowHeight))
	if f.err != nil {
		win32.DestroyWindow(hwnd)
		return nil, f.err
	}
	return w, nil
}

func (w *logWindow) handleMessage(msg uint32, wParam, lParam uintptr) (uintptr, bool) {
	switch msg {
	case win32.WM_SIZE:
		w.fitTextToWindow()
		return 0, true
	case win32.WM_CLOSE:
		w.hide()
		return 0, true
	case win32.WM_COMMAND:
		if win32.LowWord(wParam) == win32.IDCANCEL { // Escape
			w.hide()
		}
		return 0, true
	}
	return 0, false
}

// fitTextToWindow stretches the text box over the whole window after a resize.
func (w *logWindow) fitTextToWindow() {
	if w.text.hwnd == 0 {
		return
	}
	client := win32.ClientRect(w.hwnd)
	margin := w.app.scale.px(windowPadding)
	width := client.Width() - 2*margin
	height := client.Height() - 2*margin
	if width > 0 && height > 0 {
		win32.Move(w.text.hwnd, margin, margin, width, height)
	}
}

func (w *logWindow) isVisible() bool {
	return win32.IsVisible(w.hwnd)
}

// toggle shows the log window if it is hidden, and hides it if it is shown.
func (w *logWindow) toggle() {
	if w.isVisible() {
		w.hide()
		return
	}
	win32.Show(w.hwnd)
	win32.BringToFront(w.hwnd)
	w.app.main.updateLogButtonText()
}

func (w *logWindow) hide() {
	win32.Hide(w.hwnd)
	w.app.main.updateLogButtonText()
}

// logf writes a timestamped line to the log window. It is safe to call from
// any goroutine and never blocks: if the UI cannot keep up (or is closing),
// the line is dropped rather than stalling an installation.
func (a *app) logf(format string, args ...any) {
	line := time.Now().Format("15:04:05") + "  " + fmt.Sprintf(format, args...)
	select {
	case a.pendingLogLines <- line:
		a.wakeUI(wmLogLinesQueued)
	default:
	}
}

// showQueuedLines runs on the UI thread and appends the lines queued by logf.
func (w *logWindow) showQueuedLines() {
	for {
		select {
		case line := <-w.app.pendingLogLines:
			win32.AppendLine(w.text.hwnd, line)
		default:
			return
		}
	}
}
