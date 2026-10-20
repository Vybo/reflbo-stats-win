package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Vybo/reflbo-stats-win/internal/config"
	"github.com/Vybo/reflbo-stats-win/internal/source"
)

type fakeControl struct{ on bool }

func (f *fakeControl) Autostart() (bool, error)   { return f.on, nil }
func (f *fakeControl) SetAutostart(on bool) error { f.on = on; return nil }

type uiRig struct {
	ui   *UI
	hub  *Hub
	srv  *httptest.Server
	path string
	ctl  *fakeControl
}

func newUIRig(t *testing.T) *uiRig {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	cfg := testConfig()
	if err := config.Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{s: source.Sample{Status: source.OK, Items: []source.Item{
		{Key: "k:cpu", Group: "CPU [#0]", Label: "Total CPU Usage", Unit: "%", Type: "usage", Value: num(12.4)},
		{Key: "k:temp", Group: "CPU [#0]", Label: "CPU", Unit: "°C", Type: "temperature", Value: num(54.26)},
		{Key: "k:ram", Group: "System", Label: "Physical Memory Used", Unit: "MB", Type: "other", Value: num(21811)},
		{Key: "k:long", Group: "GPU [#0]", Label: "GPU Memory Controller Utilization (long label)", Unit: "%", Value: num(3)},
	}}}
	hub := NewHub(cfg, src, "DESKTOP-TEST", "1.0.0")
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)
	ctl := &fakeControl{}
	ui := &UI{Hub: hub, Path: path, Control: ctl, Addresses: func() []string { return []string{"192.168.1.20"} },
		Listen: func(port int) error {
			if port == 48001 {
				return errors.New("listen tcp :48001: bind: address already in use")
			}
			return nil
		}}
	ui.SetLAN(cfg.Port, nil)
	srv := httptest.NewServer(ui.Handler())
	t.Cleanup(func() { cancel(); srv.Close() })
	time.Sleep(50 * time.Millisecond) // the first sample
	return &uiRig{ui: ui, hub: hub, srv: srv, path: path, ctl: ctl}
}

func (r *uiRig) do(t *testing.T, method, path, body string, headers ...string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, r.srv.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i] == "Host" {
			req.Host = headers[i+1]
		} else {
			req.Header.Set(headers[i], headers[i+1])
		}
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	var buf bytes.Buffer
	buf.ReadFrom(res.Body)
	json.Unmarshal(buf.Bytes(), &out)
	return res.StatusCode, out
}

func TestTheStateHasEverythingThePageShows(t *testing.T) {
	r := newUIRig(t)
	code, st := r.do(t, "GET", "/api/state", "")
	if code != 200 {
		t.Fatalf("status %d", code)
	}
	hw := st["hwinfo"].(map[string]any)
	conn := st["connection"].(map[string]any)
	app := st["app"].(map[string]any)
	if hw["status"] != "ok" || len(st["items"].([]any)) != 4 || len(st["picked"].([]any)) != 2 ||
		conn["port"] != 47123.0 || conn["addresses"].([]any)[0] != "192.168.1.20" ||
		conn["token"] != config.Grouped(r.hub.Config().Token) ||
		app["version"] != "1.0.0" || app["autostart"] != false || app["config_path"] != r.path {
		t.Fatalf("state: %v", st)
	}
}

func TestTheStaticPageIsServed(t *testing.T) {
	r := newUIRig(t)
	res, err := http.Get(r.srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var buf bytes.Buffer
	buf.ReadFrom(res.Body)
	if res.StatusCode != 200 || !strings.Contains(buf.String(), "<title>reflbo-stats</title>") {
		t.Fatalf("page: %d %.80s", res.StatusCode, buf.String())
	}
}

func TestAPickedReadingGetsItsDefaultsAndIsSaved(t *testing.T) {
	r := newUIRig(t)
	body := `[{"key":"k:cpu","id":"cpu_load","label":"CPU load","unit":"%","scale":1,"decimals":0,"min":0,"max":100},
	          {"key":"k:ram"},{"key":"k:long"},{"key":"k:temp"}]`
	code, st := r.do(t, "PUT", "/api/readings", body)
	if code != 200 {
		t.Fatalf("status %d %v", code, st)
	}
	picked := st["picked"].([]any)
	ram := picked[1].(map[string]any)
	long := picked[2].(map[string]any)
	temp := picked[3].(map[string]any)
	if ram["id"] != "physical_memory" || ram["label"] != "Physical Memory Used" || ram["unit"] != "MB" ||
		ram["scale"] != 1.0 || ram["decimals"] != 0.0 || ram["min"] != nil {
		t.Fatalf("ram defaults: %v", ram)
	}
	if long["label"] != "GPU Memory Controller U" || long["min"] != 0.0 || long["max"] != 100.0 {
		t.Fatalf("a long label is cut at 23 bytes, a %% runs 0-100: %v", long)
	}
	if temp["id"] != "cpu" || temp["decimals"] != 1.0 {
		t.Fatalf("temperature defaults: %v", temp)
	}
	saved, err := config.Load(r.path)
	if err != nil || len(saved.Readings) != 4 {
		t.Fatalf("saved: %+v %v", saved, err)
	}
	if got := len(r.hub.Config().Readings); got != 4 {
		t.Fatalf("the hub has %d readings", got)
	}
}

func TestABadSelectionIsRefusedWithItsReasonAndNothingSaved(t *testing.T) {
	r := newUIRig(t)
	code, st := r.do(t, "PUT", "/api/readings", `[{"key":"k:cpu","id":"BAD","label":"x","unit":"%","scale":1}]`)
	if code != 400 || st["error"] != `id "BAD": 1-15 characters of a-z, 0-9 and _` {
		t.Fatalf("refusal: %d %v", code, st)
	}
	saved, _ := config.Load(r.path)
	if len(saved.Readings) != 2 {
		t.Fatalf("something was saved: %+v", saved.Readings)
	}
	if code, st := r.do(t, "PUT", "/api/readings", `{"not":"a list"}`); code != 400 || st["error"] == nil {
		t.Fatalf("not a list: %d %v", code, st)
	}
}

func TestANewTokenIsSavedAndShown(t *testing.T) {
	r := newUIRig(t)
	old := r.hub.Config().Token
	code, st := r.do(t, "POST", "/api/token", "{}")
	if code != 200 || r.hub.Config().Token == old || st["connection"].(map[string]any)["token"] != config.Grouped(r.hub.Config().Token) {
		t.Fatalf("new token: %d %v", code, st)
	}
	saved, _ := config.Load(r.path)
	if saved.Token != r.hub.Config().Token {
		t.Fatal("the new token wasn't saved")
	}
}

func TestStartWithWindowsAndThePort(t *testing.T) {
	r := newUIRig(t)
	code, st := r.do(t, "PUT", "/api/app", `{"autostart":true}`)
	if code != 200 || !r.ctl.on || st["app"].(map[string]any)["autostart"] != true {
		t.Fatalf("autostart: %d %v", code, st)
	}
	code, st = r.do(t, "PUT", "/api/app", `{"port":48000}`)
	conn := st["connection"].(map[string]any)
	if code != 200 || conn["port"] != 48000.0 || conn["listening"] != 48000.0 || conn["error"] != "" {
		t.Fatalf("a new port opens at once: %d %v", code, st)
	}
	code, st = r.do(t, "PUT", "/api/app", `{"port":48001}`)
	conn = st["connection"].(map[string]any)
	if code != 200 || conn["port"] != 48001.0 || conn["listening"] != 0.0 ||
		conn["error"] != "listen tcp :48001: bind: address already in use" {
		t.Fatalf("a taken port: %d %v", code, st)
	}
	if code, st := r.do(t, "PUT", "/api/app", `{"port":47124}`); code != 400 || st["error"] != "ports are 1024-65535 and differ" {
		t.Fatalf("the settings page's own port: %d %v", code, st)
	}
}

func TestOnlyThisPCsBrowserGetsIn(t *testing.T) {
	r := newUIRig(t)
	if code, _ := r.do(t, "GET", "/api/state", "", "Host", "evil.example:47124"); code != 403 {
		t.Fatalf("another host: %d", code)
	}
	if code, _ := r.do(t, "GET", "/api/state", "", "Host", "localhost:47124"); code != 200 {
		t.Fatalf("localhost: %d", code)
	}
	if code, _ := r.do(t, "POST", "/api/token", "{}", "Origin", "https://evil.example"); code != 403 {
		t.Fatalf("another site's page: %d", code)
	}
	req, _ := http.NewRequest("POST", r.srv.URL+"/api/token", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "text/plain")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 415 {
		t.Fatalf("a form post: %d", res.StatusCode)
	}
}

func TestANewReadingKeepsTheDecimalsItWasGiven(t *testing.T) {
	r := newUIRig(t)
	code, st := r.do(t, "PUT", "/api/readings", `[{"key":"k:temp","decimals":2},{"key":"k:ram","decimals":0}]`)
	picked := st["picked"].([]any)
	if code != 200 || picked[0].(map[string]any)["decimals"] != 2.0 || picked[1].(map[string]any)["decimals"] != 0.0 {
		t.Fatalf("decimals: %d %v", code, picked)
	}
}

func TestNoListIsAnEmptyOne(t *testing.T) {
	r := newUIRig(t)
	code, st := r.do(t, "PUT", "/api/readings", `null`)
	picked, ok := st["picked"].([]any)
	if code != 200 || !ok || len(picked) != 0 {
		t.Fatalf("null: %d %v", code, st["picked"])
	}
}

func TestThePageCantBeFramed(t *testing.T) {
	r := newUIRig(t)
	res, err := http.Get(r.srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.Header.Get("X-Frame-Options") != "DENY" || res.Header.Get("Content-Security-Policy") != "frame-ancestors 'none'" {
		t.Fatalf("headers: %v", res.Header)
	}
}

func TestAPortThatCantBeOpenedIsReported(t *testing.T) {
	r := newUIRig(t)
	r.ui.SetLAN(0, errors.New("listen tcp :47123: bind: address already in use"))
	_, st := r.do(t, "GET", "/api/state", "")
	conn := st["connection"].(map[string]any)
	if conn["listening"] != 0.0 || conn["error"] != "listen tcp :47123: bind: address already in use" {
		t.Fatalf("state: %v", conn)
	}
}

func TestAScaleLeftOutIsOneAndAZeroIsRefused(t *testing.T) {
	r := newUIRig(t)
	code, st := r.do(t, "PUT", "/api/readings", `[{"key":"k:cpu","id":"cpu_x","label":"X","unit":"%"}]`)
	if code != 200 || st["picked"].([]any)[0].(map[string]any)["scale"] != 1.0 {
		t.Fatalf("no scale: %d %v", code, st)
	}
	code, st = r.do(t, "PUT", "/api/readings", `[{"key":"k:cpu","id":"cpu_x","label":"X","unit":"%","scale":0}]`)
	if code != 400 || st["error"] != "cpu_x: the scale must be above 0" {
		t.Fatalf("a zero scale: %d %v", code, st)
	}
}
