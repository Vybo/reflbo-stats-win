// Package server is reflbo-stats' two listeners (spec §6, §7): the LAN API the boards use, and the settings
// page's API on the loopback interface. The Hub between them holds the settings, the latest sample and the boards.
package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/Vybo/reflbo-stats-win/internal/config"
	"github.com/Vybo/reflbo-stats-win/internal/source"
)

const (
	Protocol     = 1 // reflbo spec §12a
	MaxBoards    = 8
	maxFrame     = 8 * 1024
	maxDepth     = 4
	minInterval  = 250
	maxInterval  = 10000
	idleInterval = 1000 // the source's reads with no board, for the settings page
	pingEvery    = 5 * time.Second
)

// Board is a connected board, as the settings page lists it.
type Board struct {
	Addr       string    `json:"addr"`
	IntervalMS int       `json:"interval_ms"`
	Since      time.Time `json:"since"`
}

type board struct {
	Board
	mu     sync.Mutex
	conn   *websocket.Conn // nil until the upgrade
	cancel context.CancelFunc
	once   sync.Once
}

// close ends the connection once, with a code and reason the board sees; before the upgrade it only cancels.
func (b *board) close(code websocket.StatusCode, reason string) {
	b.once.Do(func() {
		b.mu.Lock()
		c := b.conn
		b.mu.Unlock()
		if c != nil {
			c.Close(code, reason)
		}
		b.cancel()
	})
}

type Hub struct {
	mu         sync.Mutex
	cfg        config.Config
	catalog    []source.Entry
	catalogVer uint64
	sample     source.Sample
	sampleVer  uint64
	boards     map[*board]bool
	src        source.Source
	name       string
	version    string
	wake       chan struct{} // the read loop's interval changed

	helloTimeout time.Duration
}

func NewHub(cfg config.Config, src source.Source, name, version string) *Hub {
	h := &Hub{cfg: cfg, catalog: source.Catalog(cfg.Readings), src: src, name: name, version: version,
		boards: map[*board]bool{}, wake: make(chan struct{}, 1), helloTimeout: 5 * time.Second}
	return h
}

// Run reads the source at the fastest interval a board asked for, until ctx ends.
func (h *Hub) Run(ctx context.Context) {
	for {
		s := h.src.Sample()
		h.mu.Lock()
		h.sample = s
		h.sampleVer++
		every := h.fastest()
		h.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-h.wake:
		case <-time.After(time.Duration(every) * time.Millisecond):
		}
	}
}

func (h *Hub) fastest() int {
	every := idleInterval
	if len(h.boards) > 0 {
		every = maxInterval
		for b := range h.boards {
			if b.IntervalMS > 0 && b.IntervalMS < every {
				every = b.IntervalMS
			}
		}
	}
	return every
}

func (h *Hub) nudge() {
	select {
	case h.wake <- struct{}{}:
	default:
	}
}

// Sample is the latest one, for the settings page.
func (h *Hub) Sample() source.Sample {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.sample
}

func (h *Hub) Config() config.Config {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.cfg
}

// SetConfig takes new settings: a new selection reaches the boards as a catalogue, a new token closes them all.
func (h *Hub) SetConfig(c config.Config) {
	h.mu.Lock()
	tokenChanged := c.Token != h.cfg.Token
	h.cfg = c
	h.catalog = source.Catalog(c.Readings)
	h.catalogVer++
	var boards []*board
	if tokenChanged {
		for b := range h.boards {
			boards = append(boards, b)
		}
	}
	h.mu.Unlock()
	for _, b := range boards {
		b.close(websocket.StatusPolicyViolation, "new token")
	}
}

// Boards lists the connected boards.
func (h *Hub) Boards() []Board {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]Board, 0, len(h.boards))
	for b := range h.boards {
		out = append(out, b.Board)
	}
	return out
}

// CloseAll ends every board's connection cleanly, so they end their sessions at once (Quit, spec §8).
func (h *Hub) CloseAll() {
	h.mu.Lock()
	var boards []*board
	for b := range h.boards {
		boards = append(boards, b)
	}
	h.mu.Unlock()
	for _, b := range boards {
		b.close(websocket.StatusGoingAway, "quit")
	}
}

// LAN is the boards' API: /v1/info and /v1/stream, both behind the token.
func (h *Hub) LAN() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/info", h.info)
	mux.HandleFunc("GET /v1/stream", h.stream)
	return h.auth(mux)
}

func (h *Hub) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		want := h.Config().Token
		if !ok || subtle.ConstantTimeCompare([]byte(config.Ungrouped(got)), []byte(want)) != 1 {
			http.Error(w, "the token is wrong", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (h *Hub) info(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	body := map[string]any{"app": "reflbo-stats", "protocol": Protocol, "version": h.version, "name": h.name,
		"hwinfo": h.sample.Status}
	h.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(body)
}

func clampInterval(ms int) int {
	return max(minInterval, min(maxInterval, ms))
}

// depth: how deeply the JSON nests, outside its strings.
func depth(b []byte) int {
	d, most, inString, escaped := 0, 0, false, false
	for _, c := range b {
		switch {
		case escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
		case c == '{' || c == '[':
			d++
			most = max(most, d)
		case c == '}' || c == ']':
			d--
		}
	}
	return most
}

type hello struct {
	Type       string `json:"type"`
	Protocol   int    `json:"protocol"`
	IntervalMS int    `json:"interval_ms"`
}

func (h *Hub) stream(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	b := &board{Board: Board{Addr: r.RemoteAddr, Since: time.Now()}, cancel: cancel}
	h.mu.Lock()
	if len(h.boards) >= MaxBoards { /* the slot is taken under the lock, so two at once can't make nine */
		h.mu.Unlock()
		http.Error(w, fmt.Sprintf("%d boards at most", MaxBoards), http.StatusServiceUnavailable)
		return
	}
	h.boards[b] = true
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.boards, b)
		h.mu.Unlock()
		b.close(websocket.StatusNormalClosure, "")
	}()
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(maxFrame)
	b.mu.Lock()
	b.conn = conn
	b.mu.Unlock()
	if ctx.Err() != nil { /* closed while upgrading (a new token, Quit) */
		conn.Close(websocket.StatusGoingAway, "")
		return
	}

	hellos := make(chan int, 4)
	go h.readLoop(ctx, conn, b, hellos)
	var interval int
	select {
	case interval = <-hellos:
	case <-time.After(h.helloTimeout):
		b.close(websocket.StatusPolicyViolation, "no hello")
		return
	case <-ctx.Done():
		return
	}
	h.writeLoop(ctx, conn, b, interval, hellos)
}

// readLoop takes the board's messages (hello, and hello again with a new interval) and answers pings.
func (h *Hub) readLoop(ctx context.Context, conn *websocket.Conn, b *board, hellos chan<- int) {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			b.close(websocket.CloseStatus(err), "")
			return
		}
		if depth(data) > maxDepth {
			b.close(websocket.StatusPolicyViolation, "too deep")
			return
		}
		var m hello
		if json.Unmarshal(data, &m) != nil || m.Type != "hello" {
			continue // unknown messages are ignored (reflbo spec §12a.3)
		}
		if m.Protocol != Protocol {
			b.close(websocket.StatusPolicyViolation, fmt.Sprintf("protocol %d", m.Protocol))
			return
		}
		every := clampInterval(m.IntervalMS)
		h.mu.Lock()
		b.IntervalMS = every
		h.mu.Unlock()
		h.nudge()
		select {
		case hellos <- every:
		default:
		}
	}
}

func (h *Hub) send(ctx context.Context, conn *websocket.Conn, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return conn.Write(wctx, websocket.MessageText, data)
}

// writeLoop sends the catalogue, HWiNFO's state and the readings at the board's interval, and pings it.
func (h *Hub) writeLoop(ctx context.Context, conn *websocket.Conn, b *board, interval int, hellos <-chan int) {
	sentCatalog := ^uint64(0)
	var sentStatus source.Status
	seq := 0
	tick := time.NewTicker(time.Duration(interval) * time.Millisecond)
	defer tick.Stop()
	ping := time.NewTicker(pingEvery)
	defer ping.Stop()
	for due := true; ; {
		if due {
			h.mu.Lock()
			cat, catVer, sample, cfg := h.catalog, h.catalogVer, h.sample, h.cfg
			h.mu.Unlock()
			if catVer != sentCatalog {
				if h.send(ctx, conn, map[string]any{"type": "catalog", "readings": cat}) != nil {
					return
				}
				sentCatalog = catVer
			}
			if sample.Status != sentStatus {
				if h.send(ctx, conn, map[string]any{"type": "status", "hwinfo": sample.Status}) != nil {
					return
				}
				sentStatus = sample.Status
			}
			seq++
			values := source.Values(cfg.Readings, sample)
			if h.send(ctx, conn, map[string]any{"type": "readings", "seq": seq, "values": values}) != nil {
				return
			}
		}
		due = false
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			due = true
		case every := <-hellos:
			tick.Reset(time.Duration(every) * time.Millisecond)
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, pingEvery)
			err := conn.Ping(pctx)
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				b.close(websocket.StatusGoingAway, "no pong")
				return
			}
		}
	}
}
