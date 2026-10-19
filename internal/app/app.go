// Package app starts reflbo-stats (spec §3): the config file, the source, the hub, and both listeners; and stops
// them. The tray in cmd/reflbo-stats is a thin layer over it, so all this runs in tests on any system.
package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/Vybo/reflbo-stats-win/internal/config"
	"github.com/Vybo/reflbo-stats-win/internal/server"
	"github.com/Vybo/reflbo-stats-win/internal/source"
)

// ErrRunning: another copy holds the settings page's port; open its page instead (spec §8, one instance).
var ErrRunning = errors.New("reflbo-stats is already running")

type Options struct {
	ConfigPath string
	Demo       bool // the demo source in place of HWiNFO's
	Version    string
	Name       string // this PC's name, for /v1/info
	Control    server.Control
	Log        *log.Logger
}

type App struct {
	Hub    *server.Hub
	UIURL  string // the settings page
	cancel context.CancelFunc
	ui     *http.Server
	done   chan struct{}
	once   sync.Once
	log    *log.Logger

	lanMu   sync.Mutex
	lan     *http.Server // nil while no LAN port is open
	lanPort int
}

// listenLAN opens the LAN API on port, closing the one before (spec §6). The boards already connected keep their
// connections; a reconnect needs the board's PC page to name the new port.
func (a *App) listenLAN(port int) error {
	a.lanMu.Lock()
	defer a.lanMu.Unlock()
	if a.lan != nil && a.lanPort == port {
		return nil
	}
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return fmt.Errorf("the LAN port %d: %w", port, err)
	}
	if a.lan != nil {
		a.lan.Close()
	}
	srv := &http.Server{Handler: a.Hub.LAN(), ReadHeaderTimeout: 10 * time.Second}
	a.lan, a.lanPort = srv, port
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.log.Printf("the LAN listener: %v", err)
		}
	}()
	a.log.Printf("boards on port %d", port)
	return nil
}

// loadConfig reads the file or its .bak; a missing or broken pair gives the defaults, written at once.
func loadConfig(path string) (config.Config, error) {
	c, err := config.LoadOrBak(path)
	if err == nil {
		return c, nil
	}
	c = config.Default()
	return c, config.Save(path, c)
}

// Addresses: this PC's IPv4 addresses on the LAN, for reflbo's PC page.
func Addresses() []string {
	var out []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() && !n.IP.IsLinkLocalUnicast() {
				out = append(out, n.IP.String())
			}
		}
	}
	return out
}

func busy(err error) bool { return errors.Is(err, syscall.EADDRINUSE) || errors.Is(err, wsaeaddrinuse) }

// Start reads the config, opens both listeners and runs; it returns once they listen.
func Start(ctx context.Context, o Options) (*App, error) {
	logger := o.Log
	if logger == nil {
		logger = log.New(os.Stderr, "reflbo-stats: ", log.LstdFlags)
	}
	cfg, err := loadConfig(o.ConfigPath)
	if err != nil {
		return nil, err
	}
	uiLn, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.UIPort))
	if err != nil {
		if busy(err) {
			return nil, fmt.Errorf("%w on port %d", ErrRunning, cfg.UIPort)
		}
		return nil, err
	}
	var src source.Source = source.NewHWiNFO()
	if o.Demo {
		src = source.NewDemo(nil)
	}
	name := o.Name
	if name == "" {
		name, _ = os.Hostname()
	}
	runCtx, cancel := context.WithCancel(ctx)
	hub := server.NewHub(cfg, src, name, o.Version)
	a := &App{Hub: hub, UIURL: fmt.Sprintf("http://127.0.0.1:%d/", cfg.UIPort), cancel: cancel, done: make(chan struct{}),
		log: logger}
	ui := &server.UI{Hub: hub, Path: o.ConfigPath, Control: o.Control, Addresses: Addresses, Listen: a.listenLAN}
	a.ui = &http.Server{Handler: ui.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go hub.Run(runCtx)
	// A LAN port another program holds doesn't stop the app: the page says why, and a new port opens at once.
	lanErr := a.listenLAN(cfg.Port)
	if lanErr != nil {
		logger.Print(lanErr)
	}
	ui.SetLAN(cfg.Port, lanErr)
	go func() {
		if err := a.ui.Serve(uiLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("the settings page: %v", err)
		}
	}()
	logger.Printf("the settings page on %s", a.UIURL)
	return a, nil
}

// Quit closes the boards' connections cleanly, then both listeners (spec §8).
func (a *App) Quit() {
	a.once.Do(func() {
		a.Hub.CloseAll()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		a.lanMu.Lock()
		if a.lan != nil {
			a.lan.Shutdown(ctx)
		}
		a.lanMu.Unlock()
		a.ui.Shutdown(ctx)
		a.cancel()
		close(a.done)
	})
}

// Listening: the LAN API is open, so the boards can reach this PC.
func (a *App) Listening() bool {
	a.lanMu.Lock()
	defer a.lanMu.Unlock()
	return a.lan != nil
}

// Done is closed once Quit has run.
func (a *App) Done() <-chan struct{} { return a.done }
