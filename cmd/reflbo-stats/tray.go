package main

import (
	"fmt"
	"log"
	"runtime"
	"time"

	"fyne.io/systray"

	"github.com/Vybo/reflbo-stats-win/internal/app"
	"github.com/Vybo/reflbo-stats-win/internal/autostart"
	"github.com/Vybo/reflbo-stats-win/internal/icon"
	"github.com/Vybo/reflbo-stats-win/internal/source"
)

func iconBytes(s icon.State) []byte {
	if runtime.GOOS == "windows" {
		return icon.ICO(s)
	}
	return icon.PNG(s)
}

// runTray shows the icon and its menu until Quit (spec §8).
func runTray(a *app.App, ctl *autostart.Control, logger *log.Logger) {
	systray.Run(func() {
		systray.SetIcon(iconBytes(icon.Normal))
		systray.SetTooltip("reflbo-stats")
		systray.SetOnTapped(func() { openBrowser(a.UIURL) })
		open := systray.AddMenuItem("Open settings", "The readings, the token and the boards")
		on, _ := ctl.Autostart()
		auto := systray.AddMenuItemCheckbox("Start with Windows", "Start reflbo-stats after you log in", on)
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "Stop serving the boards")
		go func() {
			tick := time.NewTicker(2 * time.Second)
			defer tick.Stop()
			last := icon.State(-1)
			for {
				select {
				case <-open.ClickedCh:
					openBrowser(a.UIURL)
				case <-auto.ClickedCh:
					if err := ctl.SetAutostart(!auto.Checked()); err != nil {
						logger.Printf("Start with Windows: %v", err)
					}
					if on, _ := ctl.Autostart(); on {
						auto.Check()
					} else {
						auto.Uncheck()
					}
				case <-quit.ClickedCh:
					a.Quit()
					systray.Quit()
					return
				case <-a.Done():
					systray.Quit()
					return
				case <-tick.C: // the icon follows HWiNFO and the boards; the tick follows the page's checkbox
					status, boards := a.Hub.Sample().Status, len(a.Hub.Boards())
					state := icon.Normal
					switch {
					case status != source.OK:
						state = icon.Warning
					case boards > 0:
						state = icon.Connected
					}
					if state != last {
						systray.SetIcon(iconBytes(state))
						last = state
					}
					systray.SetTooltip(fmt.Sprintf("reflbo-stats · HWiNFO %s · %d board(s)", status, boards))
					if on, _ := ctl.Autostart(); on != auto.Checked() {
						if on {
							auto.Check()
						} else {
							auto.Uncheck()
						}
					}
				}
			}
		}()
	}, a.Quit)
}
