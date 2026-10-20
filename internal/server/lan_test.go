package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Vybo/reflbo-stats-win/internal/config"
	"github.com/Vybo/reflbo-stats-win/internal/source"
)

// fakeSource returns what the test sets.
type fakeSource struct {
	mu sync.Mutex
	s  source.Sample
	n  int // Sample calls
}

func (f *fakeSource) Sample() source.Sample {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	return f.s
}

func (f *fakeSource) set(s source.Sample) {
	f.mu.Lock()
	f.s = s
	f.mu.Unlock()
}

func num(v float64) *float64 { return &v }

func items(cpu float64) source.Sample {
	return source.Sample{Status: source.OK, Items: []source.Item{
		{Key: "k:cpu", Label: "Total CPU Usage", Unit: "%", Value: num(cpu)},
		{Key: "k:temp", Label: "CPU", Unit: "°C", Value: num(54.26)},
	}}
}

func testConfig() config.Config {
	c := config.Default()
	f := func(v float64) *float64 { return &v }
	c.Readings = []config.Reading{
		{Key: "k:cpu", ID: "cpu_load", Label: "CPU load", Unit: "%", Scale: 1, Min: f(0), Max: f(100)},
		{Key: "k:temp", ID: "cpu_temp", Label: "CPU", Unit: "°C", Scale: 1, Decimals: 1},
	}
	return c
}

type rig struct {
	hub    *Hub
	src    *fakeSource
	srv    *httptest.Server
	cfg    config.Config
	cancel context.CancelFunc
}

func newRig(t *testing.T) *rig {
	t.Helper()
	src := &fakeSource{s: items(12.4)}
	cfg := testConfig()
	hub := NewHub(cfg, src, "DESKTOP-TEST", "1.0.0")
	hub.helloTimeout = 500 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)
	srv := httptest.NewServer(hub.LAN())
	t.Cleanup(func() { cancel(); srv.Close() })
	return &rig{hub: hub, src: src, srv: srv, cfg: cfg, cancel: cancel}
}

func (r *rig) url(path string) string { return r.srv.URL + path }

func (r *rig) wsURL() string { return "ws" + strings.TrimPrefix(r.srv.URL, "http") + "/v1/stream" }

func (r *rig) dial(t *testing.T, token string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return websocket.Dial(ctx, r.wsURL(), &websocket.DialOptions{HTTPHeader: http.Header{"Authorization": {"Bearer " + token}}})
}

func send(t *testing.T, c *websocket.Conn, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.Write(ctx, websocket.MessageText, b); err != nil {
		t.Fatal(err)
	}
}

type message struct {
	Type     string              `json:"type"`
	Seq      int                 `json:"seq"`
	Values   map[string]*float64 `json:"values"`
	Readings []source.Entry      `json:"readings"`
	HWiNFO   string              `json:"hwinfo"`
	at       time.Time
}

func recv(t *testing.T, c *websocket.Conn) message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, b, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var m message
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s: %v", b, err)
	}
	m.at = time.Now()
	return m
}

// recvType skips messages until one of the type.
func recvType(t *testing.T, c *websocket.Conn, typ string) message {
	t.Helper()
	for i := 0; i < 50; i++ {
		if m := recv(t, c); m.Type == typ {
			return m
		}
	}
	t.Fatalf("no %s message", typ)
	return message{}
}

func TestInfoNeedsTheToken(t *testing.T) {
	r := newRig(t)
	for _, auth := range []string{"", "Bearer WRONGWRONGWRONGWRONGWRONGW", "Basic " + r.cfg.Token} {
		req, _ := http.NewRequest("GET", r.url("/v1/info"), nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 401 {
			t.Errorf("%q: %d", auth, res.StatusCode)
		}
	}
	req, _ := http.NewRequest("GET", r.url("/v1/info"), nil)
	req.Header.Set("Authorization", "Bearer "+r.cfg.Token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var info map[string]any
	json.NewDecoder(res.Body).Decode(&info)
	if res.StatusCode != 200 || info["app"] != "reflbo-stats" || info["protocol"] != 1.0 || info["version"] != "1.0.0" ||
		info["name"] != "DESKTOP-TEST" || info["hwinfo"] != "ok" {
		t.Fatalf("info: %d %v", res.StatusCode, info)
	}
}

func TestOtherPathsAreNotFound(t *testing.T) {
	r := newRig(t)
	req, _ := http.NewRequest("GET", r.url("/v1/nope"), nil)
	req.Header.Set("Authorization", "Bearer "+r.cfg.Token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 404 {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestTheStreamNeedsTheToken(t *testing.T) {
	r := newRig(t)
	_, res, err := r.dial(t, "WRONGWRONGWRONGWRONGWRONGW")
	if err == nil || res == nil || res.StatusCode != 401 {
		t.Fatalf("want 401, got %v %v", res, err)
	}
}

func TestHelloBringsTheCatalogueTheStatusAndReadings(t *testing.T) {
	r := newRig(t)
	c, _, err := r.dial(t, r.cfg.Token)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	send(t, c, map[string]any{"type": "hello", "protocol": 1, "interval_ms": 250})
	cat := recv(t, c)
	if cat.Type != "catalog" || len(cat.Readings) != 2 || cat.Readings[0].ID != "cpu_load" || *cat.Readings[0].Max != 100 ||
		cat.Readings[1].Decimals != 1 {
		t.Fatalf("catalogue: %+v", cat)
	}
	if st := recv(t, c); st.Type != "status" || st.HWiNFO != "ok" {
		t.Fatalf("status: %+v", st)
	}
	a := recvType(t, c, "readings")
	b := recvType(t, c, "readings")
	if *a.Values["cpu_load"] != 12 || *a.Values["cpu_temp"] != 54.3 || b.Seq != a.Seq+1 {
		t.Fatalf("readings: %+v %+v", a, b)
	}
	if gap := b.at.Sub(a.at); gap < 150*time.Millisecond || gap > 600*time.Millisecond {
		t.Fatalf("readings %v apart, asked for 250 ms", gap)
	}
	if boards := r.hub.Boards(); len(boards) != 1 || boards[0].IntervalMS != 250 {
		t.Fatalf("boards: %+v", boards)
	}
}

func TestTheIntervalIsClampedTo250msAnd10s(t *testing.T) {
	if clampInterval(10) != 250 || clampInterval(60000) != 10000 || clampInterval(1000) != 1000 {
		t.Fatal("clamp")
	}
	r := newRig(t)
	c, _, _ := r.dial(t, r.cfg.Token)
	defer c.CloseNow()
	send(t, c, map[string]any{"type": "hello", "protocol": 1, "interval_ms": 10})
	a := recvType(t, c, "readings")
	b := recvType(t, c, "readings")
	if gap := b.at.Sub(a.at); gap < 150*time.Millisecond {
		t.Fatalf("readings %v apart, below the 250 ms floor", gap)
	}
}

func asClose(err error, ce *websocket.CloseError) bool { return errors.As(err, ce) }

func closedWith(t *testing.T, c *websocket.Conn) (websocket.StatusCode, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		_, _, err := c.Read(ctx)
		if err != nil {
			var ce websocket.CloseError
			if ok := asClose(err, &ce); ok {
				return ce.Code, ce.Reason
			}
			return -1, err.Error()
		}
	}
}

func TestAnotherProtocolIsRefusedWithItsNumber(t *testing.T) {
	r := newRig(t)
	c, _, _ := r.dial(t, r.cfg.Token)
	defer c.CloseNow()
	send(t, c, map[string]any{"type": "hello", "protocol": 2, "interval_ms": 1000})
	if code, reason := closedWith(t, c); code != websocket.StatusPolicyViolation || reason != "protocol 2" {
		t.Fatalf("closed %d %q", code, reason)
	}
}

func TestABoardThatSaysNothingIsClosed(t *testing.T) {
	r := newRig(t)
	c, _, _ := r.dial(t, r.cfg.Token)
	defer c.CloseNow()
	if code, reason := closedWith(t, c); code != websocket.StatusPolicyViolation || reason != "no hello" {
		t.Fatalf("closed %d %q", code, reason)
	}
}

func TestOversizedOrDeepMessagesClose(t *testing.T) {
	r := newRig(t)
	big, _, _ := r.dial(t, r.cfg.Token)
	defer big.CloseNow()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	big.Write(ctx, websocket.MessageText, []byte(`{"type":"hello","pad":"`+strings.Repeat("x", 9000)+`"}`))
	if code, _ := closedWith(t, big); code != websocket.StatusMessageTooBig {
		t.Fatalf("a 9 KB frame: closed %d", code)
	}
	deep, _, _ := r.dial(t, r.cfg.Token)
	defer deep.CloseNow()
	deep.Write(ctx, websocket.MessageText, []byte(`{"type":"hello","a":{"b":{"c":{"d":{}}}}}`))
	if code, reason := closedWith(t, deep); code != websocket.StatusPolicyViolation || reason != "too deep" {
		t.Fatalf("five levels: closed %d %q", code, reason)
	}
}

func TestANewSelectionSendsTheCatalogueAgain(t *testing.T) {
	r := newRig(t)
	c, _, _ := r.dial(t, r.cfg.Token)
	defer c.CloseNow()
	send(t, c, map[string]any{"type": "hello", "protocol": 1, "interval_ms": 250})
	recvType(t, c, "readings")
	next := r.cfg
	next.Readings = next.Readings[:1]
	r.hub.SetConfig(next)
	cat := recvType(t, c, "catalog")
	if len(cat.Readings) != 1 {
		t.Fatalf("catalogue: %+v", cat)
	}
	if m := recvType(t, c, "readings"); len(m.Values) != 1 {
		t.Fatalf("readings after: %+v", m)
	}
}

func TestHWiNFOsStateIsSentWhenItChanges(t *testing.T) {
	r := newRig(t)
	c, _, _ := r.dial(t, r.cfg.Token)
	defer c.CloseNow()
	send(t, c, map[string]any{"type": "hello", "protocol": 1, "interval_ms": 250})
	recvType(t, c, "readings")
	stale := items(50)
	stale.Status = source.Stale
	r.src.set(stale)
	if st := recvType(t, c, "status"); st.HWiNFO != "stale" {
		t.Fatalf("status: %+v", st)
	}
	if m := recvType(t, c, "readings"); m.Values["cpu_load"] != nil {
		t.Fatalf("values while stale: %v", *m.Values["cpu_load"])
	}
}

func TestANewTokenClosesEveryBoard(t *testing.T) {
	r := newRig(t)
	c, _, _ := r.dial(t, r.cfg.Token)
	defer c.CloseNow()
	send(t, c, map[string]any{"type": "hello", "protocol": 1, "interval_ms": 250})
	recvType(t, c, "readings")
	next := r.cfg
	next.Token = config.NewToken()
	r.hub.SetConfig(next)
	if code, reason := closedWith(t, c); code != websocket.StatusPolicyViolation || reason != "new token" {
		t.Fatalf("closed %d %q", code, reason)
	}
	if _, res, err := r.dial(t, r.cfg.Token); err == nil || res.StatusCode != 401 {
		t.Fatal("the old token still works")
	}
	c2, _, err := r.dial(t, next.Token)
	if err != nil {
		t.Fatalf("the new token: %v", err)
	}
	c2.CloseNow()
}

func TestANinthBoardIsTurnedAway(t *testing.T) {
	r := newRig(t)
	var conns []*websocket.Conn
	for i := 0; i < MaxBoards; i++ {
		c, _, err := r.dial(t, r.cfg.Token)
		if err != nil {
			t.Fatal(err)
		}
		send(t, c, map[string]any{"type": "hello", "protocol": 1, "interval_ms": 1000})
		recvType(t, c, "catalog")
		conns = append(conns, c)
	}
	defer func() {
		for _, c := range conns {
			c.CloseNow()
		}
	}()
	if _, res, err := r.dial(t, r.cfg.Token); err == nil || res.StatusCode != 503 {
		t.Fatalf("a ninth board: %v %v", res, err)
	}
}

func TestQuitClosesTheBoardsAtOnce(t *testing.T) {
	r := newRig(t)
	c, _, _ := r.dial(t, r.cfg.Token)
	defer c.CloseNow()
	send(t, c, map[string]any{"type": "hello", "protocol": 1, "interval_ms": 1000})
	recvType(t, c, "catalog")
	r.hub.CloseAll()
	if code, _ := closedWith(t, c); code != websocket.StatusGoingAway {
		t.Fatalf("closed %d", code)
	}
}

func TestTheSourceIsReadAtTheFastestBoardsInterval(t *testing.T) {
	r := newRig(t)
	time.Sleep(1200 * time.Millisecond)
	r.src.mu.Lock()
	idle := r.src.n
	r.src.n = 0
	r.src.mu.Unlock()
	if idle < 1 || idle > 3 { /* once a second with no board */
		t.Fatalf("%d reads in 1.2 s with no board", idle)
	}
	c, _, _ := r.dial(t, r.cfg.Token)
	defer c.CloseNow()
	send(t, c, map[string]any{"type": "hello", "protocol": 1, "interval_ms": 250})
	recvType(t, c, "readings")
	r.src.mu.Lock()
	r.src.n = 0
	r.src.mu.Unlock()
	time.Sleep(1000 * time.Millisecond)
	r.src.mu.Lock()
	busy := r.src.n
	r.src.mu.Unlock()
	if busy < 3 {
		t.Fatalf("%d reads in 1 s with a 250 ms board", busy)
	}
}

func TestAScaleThatOverflowsSendsNullAndKeepsTheBoard(t *testing.T) {
	r := newRig(t)
	c, _, _ := r.dial(t, r.cfg.Token)
	defer c.CloseNow()
	next := r.cfg
	next.Readings = append([]config.Reading{}, next.Readings...)
	next.Readings[1].Scale = 1e306
	next.Readings[1].Decimals = 3
	r.hub.SetConfig(next)
	send(t, c, map[string]any{"type": "hello", "protocol": 1, "interval_ms": 250})
	a := recvType(t, c, "readings")
	b := recvType(t, c, "readings")
	if a.Values["cpu_temp"] != nil || *b.Values["cpu_load"] != 12 {
		t.Fatalf("readings: %+v %+v", a, b)
	}
}
