// squelite is a native SQLite browser: it opens database files, shows their
// schema, and edits table rows in place.
//
//	squelite [database...]
package main

import (
	"log"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	app := NewApp()
	mygo.App.WhenReady(func() {
		app.win = mygo.NewWindow(mygo.WindowOptions{
			Title:   "squelite",
			Width:   1200,
			Height:  800,
			Content: ui.View(app.view),
		})
		for _, path := range os.Args[1:] {
			go app.openDatabase(path)
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
