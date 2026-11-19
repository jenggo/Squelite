package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// logEntry is one line of the status dock.
type logEntry struct {
	db   string
	text string
}

// logf appends a message to the status log, prefixed with the database it
// concerns, and asks for a frame. It is safe from any goroutine.
func (a *App) logf(db, format string, args ...any) {
	e := logEntry{db: db, text: fmt.Sprintf(format, args...)}
	a.update(func() { a.appendLog(e) })
}

// appendLog adds a line to the log on the main thread.
func (a *App) appendLog(e logEntry) {
	a.log = append(a.log, e)
	if len(a.log) > 500 {
		a.log = a.log[len(a.log)-500:]
	}
	a.wake()
}

// statusDock draws the log at the bottom of the window: one line until the
// user drags its top edge up.
func (a *App) statusDock(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Height(a.statusHeight).Shrink(0).Background(t.Surface).Border(1, t.Border).Children(func() {
		handle := ui.Row(c).Height(4).FillWidth().Cursor(ui.CursorResizeNS).Background(t.Border.Alpha(0.4)).Label("Resize status")
		if handle.Hovered() {
			handle.Background(t.Border)
		}
		if _, dy, ok := handle.Dragged(); ok {
			a.statusHeight = min(max(a.statusHeight-dy, 26), 400)
		}
		ui.Row(c).Padding(0, 10).Gap(12).Children(func() {
			if len(a.log) > 0 {
				ui.Text(c, logLine(a.log[len(a.log)-1])).Font("monospace").FontSize(12).SingleLine().Grow(1)
			} else {
				ui.Spacer(c)
			}
			if a.active != nil {
				ui.Textf(c, "%d rows · %d columns", a.active.count, len(a.active.columns)).FontSize(12).TextColor(t.TextMuted)
			}
		})
		if a.statusHeight <= 30 {
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Fill().Padding(0, 10).Gap(1).Children(func() {
				for _, e := range a.log {
					ui.Text(c, logLine(e)).Font("monospace").FontSize(12).SingleLine()
				}
			})
		})
	})
}

func logLine(e logEntry) string {
	if e.db == "" {
		return e.text
	}
	return e.db + ": " + e.text
}
