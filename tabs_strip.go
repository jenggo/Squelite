package main

import "github.com/egoist/mygo/ui"

func (a *App) tabStrip(c *ui.Context, tabs []*Tab) {
	t := c.Theme()
	parts := ui.TabsBase(c, &a.tabIndex, len(tabs))
	parts.List.Gap(2).Padding(6, 8).Children(func() {
		for i, tab := range tabs {
			el := parts.Tab(i).Padding(4, 8).Radius(6)
			if i == a.tabIndex {
				el.Background(t.Surface)
			}
			el.Children(func() {
				ui.Row(c).Gap(6).Children(func() {
					ui.Text(c, tab.db.name+" · "+tab.table).SingleLine()
					closeBtn := ui.ButtonBase(c).Label("Close "+tab.table).Padding(0, 4).Radius(4)
					hovered := closeBtn.Hovered()
					if hovered {
						closeBtn.Background(t.Danger.Alpha(0.15))
					}
					closeBtn.Children(func() {
						color := t.TextMuted
						if hovered {
							color = t.Danger
						}
						ui.Text(c, "×").FontSize(12).TextColor(color)
					})
					closeBtn.OnClick(func() { a.closeTab(tab) })
				})
			})
		}
	})
}
