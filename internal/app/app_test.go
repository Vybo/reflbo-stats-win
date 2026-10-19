package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Vybo/reflbo-stats-win/internal/config"
)

type offControl struct{}

func (offControl) Autostart() (bool, error) { return false, nil }
func (offControl) SetAutostart(bool) error  { return errors.New("not here") }

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

// started: an app with the demo source on free ports, its config written first.
func started(t *testing.T) (*App, config.Config, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Port, cfg.UIPort = freePort(t), freePort(t)
	cfg.Readings = []config.Reading{{Key: "demo:cpu:load", ID: "cpu_load", Label: "CPU load", Unit: "%", Scale: 1}}
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	a, err := Start(context.Background(), Options{ConfigPath: path, Demo: true, Version: "1.0.0", Name: "DESKTOP-TEST",
		Control: offControl{}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Quit)
	return a, cfg, path
}

func TestTheAppServesThePageAndTheBoards(t *testing.T) {
	a, cfg, _ := started(t)
	if a.UIURL != fmt.Sprintf("http://127.0.0.1:%d/", cfg.UIPort) {
		t.Fatalf("UI URL %s", a.UIURL)
	}
	res, err := http.Get(a.UIURL + "api/state")
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		HWiNFO struct{ Status string } `json:"hwinfo"`
		Items  []any                   `json:"items"`
	}
	json.NewDecoder(res.Body).Decode(&st)
	res.Body.Close()
	if st.HWiNFO.Status != "ok" || len(st.Items) < 8 {
		t.Fatalf("the demo's state: %+v", st)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/v1/stream", cfg.Port),
		&websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + cfg.Token}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	c.Write(ctx, websocket.MessageText, []byte(`{"type":"hello","protocol":1,"interval_ms":250}`))
	_, b, err := c.Read(ctx)
	if err != nil || !json.Valid(b) {
		t.Fatalf("first message: %s %v", b, err)
	}
	var m struct{ Type string }
	json.Unmarshal(b, &m)
	if m.Type != "catalog" {
		t.Fatalf("first message %s", b)
	}
}

func TestASecondStartFindsTheFirst(t *testing.T) {
	_, _, path := started(t)
	_, err := Start(context.Background(), Options{ConfigPath: path, Demo: true, Control: offControl{}})
	if !errors.Is(err, ErrRunning) {
		t.Fatalf("want ErrRunning, got %v", err)
	}
}

func TestQuitClosesTheBoardsAtOnce(t *testing.T) {
	a, cfg, _ := started(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/v1/stream", cfg.Port),
		&websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + cfg.Token}}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	c.Write(ctx, websocket.MessageText, []byte(`{"type":"hello","protocol":1,"interval_ms":1000}`))
	c.Read(ctx) // the catalogue
	start := time.Now()
	go a.Quit()
	for {
		if _, _, err := c.Read(ctx); err != nil {
			if websocket.CloseStatus(err) != websocket.StatusGoingAway {
				t.Fatalf("closed with %v", err)
			}
			break
		}
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Quit took %v to reach the board", d)
	}
	select {
	case <-a.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("the app didn't stop")
	}
}

func TestAMissingConfigIsWrittenWithItsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reflbo-stats", "config.json")
	cfg, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := config.Load(path)
	if err != nil || saved.Token != cfg.Token || saved.Port != 47123 {
		t.Fatalf("saved: %+v %v", saved, err)
	}
}

func TestThisPCsAddressesLeaveOutLoopback(t *testing.T) {
	for _, a := range Addresses() {
		if ip := net.ParseIP(a); ip == nil || ip.IsLoopback() || ip.To4() == nil {
			t.Fatalf("address %q", a)
		}
	}
}

func TestATakenLANPortKeepsThePageAndCanBeChanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := config.Default()
	cfg.Port, cfg.UIPort = freePort(t), freePort(t)
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	taken, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.Port)) // another program holds the LAN port
	if err != nil {
		t.Fatal(err)
	}
	defer taken.Close()
	a, err := Start(context.Background(), Options{ConfigPath: path, Demo: true, Control: offControl{}})
	if err != nil {
		t.Fatalf("the app gave up: %v", err)
	}
	defer a.Quit()
	state := func() map[string]any {
		res, err := http.Get(a.UIURL + "api/state")
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var st map[string]any
		json.NewDecoder(res.Body).Decode(&st)
		return st["connection"].(map[string]any)
	}
	if c := state(); c["listening"] != 0.0 || c["error"] == "" {
		t.Fatalf("a taken port isn't reported: %v", c)
	}
	free := freePort(t)
	req, _ := http.NewRequest("PUT", a.UIURL+"api/app", strings.NewReader(fmt.Sprintf(`{"port":%d}`, free)))
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if c := state(); c["listening"] != float64(free) || c["error"] != "" {
		t.Fatalf("the new port: %v", c)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, fmt.Sprintf("ws://127.0.0.1:%d/v1/stream", free),
		&websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + cfg.Token}}})
	if err != nil {
		t.Fatalf("a board on the new port: %v", err)
	}
	c.CloseNow()
}
