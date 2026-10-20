// reflbo-stats serves this PC's HWiNFO sensors to reflbo boards on the LAN (spec §1). A tray app on Windows:
// left-click opens the settings page; right-click has Open settings, Start with Windows and Quit.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Vybo/reflbo-stats-win/internal/app"
	"github.com/Vybo/reflbo-stats-win/internal/autostart"
	"github.com/Vybo/reflbo-stats-win/internal/config"
	"github.com/Vybo/reflbo-stats-win/internal/hwinfo"
)

var version = "dev" // set by the build: -ldflags "-X main.version=1.0.0"

const maxLog = 1 << 20

// quiet passes writes on and swallows their errors: the windowed exe's stderr has no handle, and a failing
// leg must not stop the file's.
type quiet struct{ w io.Writer }

func (q quiet) Write(p []byte) (int, error) {
	q.w.Write(p)
	return len(p), nil
}

// logWriter writes to the file first, then to stderr where there is one.
func logWriter(file, stderr io.Writer) io.Writer {
	return io.MultiWriter(file, quiet{stderr})
}

// openLog appends to log.txt beside the settings, starting it afresh past 1 MB; the exe has no console.
func openLog(configPath string) *log.Logger {
	path := filepath.Join(filepath.Dir(configPath), "log.txt")
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if st, err := os.Stat(path); err == nil && st.Size() > maxLog {
		flags |= os.O_TRUNC
	}
	os.MkdirAll(filepath.Dir(path), 0o700)
	var w io.Writer = os.Stderr
	if f, err := os.OpenFile(path, flags, 0o600); err == nil {
		w = logWriter(f, os.Stderr)
	}
	return log.New(w, "", log.LstdFlags)
}

func main() {
	demo := flag.Bool("demo", false, "serve the demo source in place of HWiNFO's")
	dump := flag.String("dump", "", "write HWiNFO's shared memory to this file once, for the tests, and exit")
	noTray := flag.Bool("no-tray", false, "run without the tray icon; Ctrl+C quits")
	configPath := flag.String("config", "", `the settings file (default: %APPDATA%\reflbo-stats\config.json)`)
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("reflbo-stats", version)
		return
	}
	path := *configPath
	if path == "" {
		var err error
		if path, err = config.Path(); err != nil {
			fmt.Fprintln(os.Stderr, "reflbo-stats:", err)
			os.Exit(1)
		}
	}
	logger := openLog(path) // the windowed exe has no console: log.txt says what happened
	if *dump != "" {
		b, err := hwinfo.ReadMapping()
		if err == nil {
			err = os.WriteFile(*dump, b, 0o600)
		}
		if err != nil {
			logger.Fatalf("-dump: %v", err)
		}
		logger.Printf("-dump: wrote %d bytes to %s", len(b), *dump)
		return
	}
	exe, _ := os.Executable()
	ctl := autostart.New(exe)
	a, err := app.Start(context.Background(), app.Options{ConfigPath: path, Demo: *demo, Version: version, Control: ctl, Log: logger})
	if errors.Is(err, app.ErrRunning) { // one instance: show the running one's page
		if c, err := config.LoadOrBak(path); err == nil {
			openBrowser(fmt.Sprintf("http://127.0.0.1:%d/", c.UIPort))
		}
		logger.Print(err)
		return
	}
	if err != nil {
		logger.Fatal(err)
	}
	if *noTray { // for tests: no tray, no browser
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
		<-stop
		a.Quit()
		return
	}
	if len(a.Hub.Config().Readings) == 0 || !a.Listening() { // a first start, or a LAN port that's taken
		openBrowser(a.UIURL)
	}
	runTray(a, ctl, logger)
}
