package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/Vybo/reflbo-stats-win/internal/config"
	"github.com/Vybo/reflbo-stats-win/internal/source"
	"github.com/Vybo/reflbo-stats-win/web"
)

// Control is what the page can change outside the config file: Start with Windows (spec §8).
type Control interface {
	Autostart() (bool, error)
	SetAutostart(on bool) error
}

// UI is the settings page and its API, served on the loopback interface only (spec §7).
type UI struct {
	Hub       *Hub
	Path      string // the config file
	Control   Control
	Addresses func() []string // this PC's LAN addresses, for reflbo's PC page
	// Listen opens the LAN API on a new port at once, closing the old one; the app's (internal/app)
	Listen func(port int) error

	mu        sync.Mutex
	listening int    // the LAN port the boards reach now; 0 when it couldn't be opened
	lanError  string // why it couldn't
}

// SetLAN reports the LAN listener's state: its port, or why it isn't listening (spec §6, §11).
func (u *UI) SetLAN(port int, err error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.listening, u.lanError = port, ""
	if err != nil {
		u.listening, u.lanError = 0, err.Error()
	}
}

const maxBody = 64 * 1024

func (u *UI) Handler() http.Handler {
	mux := http.NewServeMux()
	static, _ := fs.Sub(web.Files, ".")
	mux.Handle("GET /", http.FileServerFS(static))
	mux.HandleFunc("GET /api/state", u.state)
	mux.HandleFunc("PUT /api/readings", u.putReadings)
	mux.HandleFunc("POST /api/token", u.newToken)
	mux.HandleFunc("PUT /api/app", u.putApp)
	return guard(mux)
}

// loopback: a host name or address of this PC's own loopback.
func loopback(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// guard keeps out what isn't this PC's own browser on this page: another Host (DNS rebinding), another site's
// page (Origin), and changes sent as anything but JSON, which another site's form can't send without asking.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopback(r.Host) {
			http.Error(w, "this page answers on 127.0.0.1 only", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" {
			if ou, err := url.Parse(o); err != nil || !loopback(ou.Host) {
				http.Error(w, "another site's page", http.StatusForbidden)
				return
			}
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
				http.Error(w, "send JSON", http.StatusUnsupportedMediaType)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		}
		w.Header().Set("X-Frame-Options", "DENY") // no other page frames this one (clickjacking New token)
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func reply(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func refuse(w http.ResponseWriter, err error) {
	reply(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
}

func (u *UI) snapshot() map[string]any {
	cfg := u.Hub.Config()
	sample := u.Hub.Sample()
	autostart, _ := u.Control.Autostart()
	u.mu.Lock()
	listening, lanError := u.listening, u.lanError
	u.mu.Unlock()
	items := sample.Items
	if items == nil {
		items = []source.Item{}
	}
	return map[string]any{
		"hwinfo": map[string]any{"status": sample.Status, "detail": sample.Detail},
		"items":  items,
		"picked": cfg.Readings,
		"boards": u.Hub.Boards(),
		"connection": map[string]any{"addresses": u.Addresses(), "port": cfg.Port, "listening": listening,
			"error": lanError, "ui_port": cfg.UIPort, "token": config.Grouped(cfg.Token)},
		"app": map[string]any{"version": u.Hub.version, "name": u.Hub.name, "autostart": autostart,
			"config_path": u.Path},
	}
}

func (u *UI) state(w http.ResponseWriter, r *http.Request) {
	reply(w, http.StatusOK, u.snapshot())
}

func (u *UI) save(c config.Config) error {
	if err := config.Save(u.Path, c); err != nil {
		return err
	}
	u.Hub.SetConfig(c)
	return nil
}

// cutBytes: s cut to n bytes at a character.
func cutBytes(s string, n int) string {
	for len(s) > n {
		_, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
	}
	return s
}

// incoming is a reading as the page sends it: one may leave out its decimals or its scale, which differs from 0.
type incoming struct {
	config.Reading
	Decimals *int       `json:"decimals"`
	Scale    scaleField `json:"scale"`
}

// scaleField tells a scale left out (1) from one sent as null, which is a page that couldn't read what was typed.
type scaleField struct {
	set, null bool
	v         float64
}

func (f *scaleField) UnmarshalJSON(b []byte) error {
	f.set = true
	if string(b) == "null" {
		f.null = true
		return nil
	}
	return json.Unmarshal(b, &f.v)
}

// withDefaults fills what a newly ticked reading leaves out from what HWiNFO says of it (spec §5).
func withDefaults(in incoming, item *source.Item, taken map[string]bool) config.Reading {
	r := in.Reading
	if in.Decimals != nil {
		r.Decimals = *in.Decimals
	}
	r.Scale = 1 // left out: as HWiNFO has it; a 0 sent is refused by Validate
	if in.Scale.set {
		r.Scale = in.Scale.v
	}
	if item != nil {
		if r.Label == "" {
			r.Label = cutBytes(item.Label, config.MaxLabelLen)
		}
		if r.Unit == "" {
			r.Unit = cutBytes(item.Unit, config.MaxUnitLen)
		}
	}
	if r.ID == "" {
		r.ID = config.MakeID(r.Label, taken)
		switch {
		case in.Decimals != nil: // the page's own
		case r.Unit == "%", r.Unit == "MHz", r.Unit == "MB", r.Unit == "RPM", r.Unit == "KB/s", r.Unit == "MB/s":
			r.Decimals = 0
		case r.Unit == "V":
			r.Decimals = 3
		default:
			r.Decimals = 1
		}
		if r.Unit == "%" && r.Min == nil && r.Max == nil {
			lo, hi := 0.0, 100.0
			r.Min, r.Max = &lo, &hi
		}
	}
	taken[r.ID] = true
	return r
}

func (u *UI) putReadings(w http.ResponseWriter, r *http.Request) {
	var sent []incoming
	if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
		refuse(w, errors.New("send a list of readings"))
		return
	}
	byKey := map[string]*source.Item{}
	sample := u.Hub.Sample()
	for i := range sample.Items {
		byKey[sample.Items[i].Key] = &sample.Items[i]
	}
	taken := map[string]bool{}
	for _, p := range sent {
		if p.ID != "" {
			taken[p.ID] = true
		}
	}
	for _, in := range sent {
		if in.Scale.null {
			id := in.ID
			if id == "" {
				id = in.Key
			}
			refuse(w, fmt.Errorf("%s: the scale is not a number", id))
			return
		}
	}
	picked := make([]config.Reading, len(sent)) // never nil: null is an empty list
	for i := range sent {
		picked[i] = withDefaults(sent[i], byKey[sent[i].Key], taken)
	}
	c := u.Hub.Config()
	c.Readings = picked
	if err := config.Validate(c); err != nil {
		refuse(w, err)
		return
	}
	if err := u.save(c); err != nil {
		refuse(w, err)
		return
	}
	reply(w, http.StatusOK, u.snapshot())
}

func (u *UI) newToken(w http.ResponseWriter, r *http.Request) {
	c := u.Hub.Config()
	c.Token = config.NewToken()
	if err := u.save(c); err != nil {
		refuse(w, err)
		return
	}
	reply(w, http.StatusOK, u.snapshot())
}

func (u *UI) putApp(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Autostart *bool `json:"autostart"`
		Port      *int  `json:"port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		refuse(w, errors.New("send the settings as JSON"))
		return
	}
	if body.Port != nil {
		c := u.Hub.Config()
		c.Port = *body.Port
		if err := config.Validate(c); err != nil {
			refuse(w, err)
			return
		}
		if err := u.save(c); err != nil {
			refuse(w, err)
			return
		}
		if u.Listen != nil { // the new port at once; the page says why when it can't be opened
			u.SetLAN(c.Port, u.Listen(c.Port))
		}
	}
	if body.Autostart != nil {
		if err := u.Control.SetAutostart(*body.Autostart); err != nil {
			refuse(w, err)
			return
		}
	}
	reply(w, http.StatusOK, u.snapshot())
}
