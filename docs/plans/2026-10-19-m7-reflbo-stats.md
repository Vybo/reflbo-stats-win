# M7 (PC half): reflbo-stats, the Windows Tray App, Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** reflbo-stats (reflbo D44, M7's PC half): a Windows tray app that reads HWiNFO Pro's shared memory and serves the readings the owner picks on its own settings page to reflbo boards on the LAN, by WebSocket with a token, as reflbo's spec §12a defines.

**Architecture:**

- **Pure Go, tested on any system:**
  - `config`: the settings file, the token, reading ids.
  - `hwinfo.Parse`: HWiNFO's shared-memory layout.
  - `source`: HWiNFO's states, the demo PC, the catalogue and the values the boards get.
  - `server`: the hub, the LAN API and the settings page's API.
  - `app`: start and stop, one instance.
  - `icon`: the tray's icons.
- **Windows-only and thin, behind build tags:**
  - opening the mapping under HWiNFO's mutex;
  - telling "shm off" from "not running";
  - the per-user Run key;
  - Windows' "address in use".
- **The page:** plain HTML, CSS and JS, embedded in the exe; its tests run in Node against a fake app.
- **The tray:** a thin layer in `main` over `internal/app`.

**Tech stack:** Go 1.26 or later, as `golang.org/x/sys` v0.49 needs it (1.27.2 on the owner's Mac, from Homebrew); `fyne.io/systray`, `github.com/coder/websocket`, `golang.org/x/sys`; Node 22's test runner for the page; headless Chrome for its screenshot.

**Spec:** `docs/spec.md` (r2, as built) in this repository. The protocol, which binds both sides, is reflbo's design spec §12a, `docs/specs/2026-09-25-firmware-design.md` in `Vybo/reflbo` (r52); also its D44 (§1.2) and M7 (§18). Where they disagree on the protocol, reflbo's wins.
- Relevant here: §3 (architecture), §4 (reading HWiNFO), §5 (the picked readings), §6 (the LAN API), §7 (the settings page), §8 (the tray), §9 (configuration), §10 (tests and checks), §11 (risks).
- Also this repository's `AGENTS.md` (commands, rules, gotchas).

**How this plan was made:**
- **Built first.** Its code was built and tested task by task on the branch `plan/m7` from `main` (4a80603), one commit per task (`plan-m7 task N: …`), and pushed. The plan carries that code verbatim, and each step's Expected output is what the step printed there.
- **Tests and sizes as built:**
  - `go test -race ./...` passes in all eight packages, and `go vet` is clean for macOS and for Windows;
  - the page has 14 tests;
  - `dist/reflbo-stats.exe` is 8 139 776 bytes (stripped, `-H windowsgui`);
  - the settings page was screenshot against the demo source.
- **The review (2026-10-19).** A fresh reviewer read the whole branch and probed the demo:
  - with a WebSocket client: 20 boards connecting at once, a silent board, a slow one;
  - with curl: rebinding Hosts, foreign Origins, preflights;
  - in headless Chrome.

  It found no Critical issue and seven Important ones. All were fixed test first in their tasks, with three cheap minors that would trip the owner:
  - HWiNFO's mutex taken without locking the OS thread (Task 2).
  - A non-finite number, HWiNFO's own or a scale's overflow, closing every board and emptying the page (Task 3).
  - Staleness that ignored HWiNFO's polling period (Tasks 2 and 3).
  - A taken LAN port making the app exit without a word (Tasks 5 and 6).
  - Every edit redrawing the table, which lost the first Save click and the keyboard's focus (Task 5).
  - The search box keeping one letter (Task 5).
  - A new reading's decimals overwritten (Task 5).
  - The three minors: `readings: null` breaking the page (Tasks 1 and 5); framing the page (clickjacking New token; Task 5); `-dump` reporting nowhere from the windowed exe (Task 6).

  A scale left out read as 0 and was refused; it is 1 now (Task 5). The other minors are in the spec's §12.
- **Not checked yet:** everything on Windows itself.
  - HWiNFO Pro's real mapping (the parser follows HWiNFO's published layout; the owner's `-dump` confirms it).
  - The tray, Start with Windows, SmartScreen, the firewall.
  - A board connecting from the LAN.

  Those are the owner's checks (§10).

## Global Constraints

- Windows 10 or 11, 64-bit; HWiNFO **Pro** with Shared Memory Support on (D44); the free HWiNFO's 12 h limit isn't worked around.
- Go 1.26 or later; dependencies `fyne.io/systray`, `github.com/coder/websocket`, `golang.org/x/sys` and nothing else beyond the standard library (spec §3).
- **The protocol** (reflbo §12a):
  - `protocol` 1; `GET /v1/info` and the WebSocket `/v1/stream`, both behind `Authorization: Bearer <token>`.
  - Messages: hello, catalog, readings, status.
  - The interval 250 ms–10 s, set by the board.
  - Pings every 5 s; frames of at most 8 KB; JSON at most 4 levels deep.
  - At most 32 readings: ids of 1–15 characters of a-z, 0-9 and _, labels of at most 23 bytes, units of at most 7, decimals 0–3.
  - `null` for a value HWiNFO can't deliver.
- **Ports:** 47123 for the LAN API on all interfaces; 47124 for the settings page on 127.0.0.1 only (spec §6, §7).
- **The token:** 128 random bits as 26 base32 characters, shown in groups of five; never logged, never committed.
- **The settings file:** `%APPDATA%\reflbo-stats\config.json`, written atomically with a `.bak` (spec §9).
- **Start with Windows:** off by default; the per-user Run key, no administrator rights (D44, spec §8).
- **The tray:** left-click opens the settings; right-click has Open settings, Start with Windows and Quit; Quit closes the boards' connections at once (spec §8).
- Apache-2.0; hwinfo-streamdeck (GPL-3.0) is a reference, never a source of code.
- No AI or assistant attribution anywhere: commits, code comments, docs.

## Review Focus

1. **A board that vanishes without a word**: Wi-Fi lost, a reset, a deep sleep mid-session. Expected: the app's ping notices within about 10 s, frees the board's slot and its goroutines, and a board that comes back connects again. Pinned by: none of the tests here (a half-open TCP needs a network to cut), so the reviewer's probe stands for it (the silent board left the list after 15 s), with the owner's check 6 and reflbo's own board checks.
2. **HWiNFO going away and coming back while boards are connected**: closed, restarted, its 12 hours up, Shared Memory Support turned off. Expected: the boards get the new state and `null` values at once, and the same keys pick up again when HWiNFO returns. Pinned by: `TestHWiNFOsStates`, `TestHWiNFOIsStaleWhenItsPollTimeStopsFor5s` and `TestStaleWaitsThreeOfHWiNFOsPollingPeriods` (Task 3); `TestHWiNFOsStateIsSentWhenItChanges` (Task 4).
3. **The owner changing the list while boards are connected**: a reading ticked, removed, renamed or rescaled, a new token. Expected: each board gets the new catalogue at its next interval; a new token closes them all with that reason, and the old one stops working. Pinned by: `TestANewSelectionSendsTheCatalogueAgain`, `TestANewTokenClosesEveryBoard` (Task 4); the page's ticking and saving tests (Task 5).
4. **A hostile or broken client on the LAN**: no token, a wrong one, another protocol, a frame of 9 KB, nested JSON, twenty connections at once, a number no JSON can carry. Expected: each is refused with a reason or a code, nothing crashes or leaks, and the other boards carry on. Pinned by: the refusal tests in `lan_test.go` (Task 4), `TestANonFiniteNumberIsNone` (Task 3), `TestAScaleThatOverflowsSendsNullAndKeepsTheBoard` (Task 4); the reviewer's 20 concurrent connects.
5. **Another site's page in the owner's own browser**: a rebinding host name, a cross-site form, a framed page. Expected: it can't read the token or change anything. Pinned by: `TestOnlyThisPCsBrowserGetsIn`, `TestThePageCantBeFramed` (Task 5).

## Rulings this plan makes

- **A layout the reader refuses** (an unknown version, short elements) reads as "not running" for the boards, with the reason on the page: the protocol (reflbo §12a) has four states and no fifth.
- **Defaults** for a newly ticked reading are filled in by the app on Save, not by the page: one implementation of the id rule, the decimals by unit and the % range.
- **Accents** in ids are dropped with a small table rather than `golang.org/x/text`, which would add a fourth dependency (spec §3).
- **HWiNFO's known layout versions** are 1 and 2, to be confirmed by the owner's `-dump`; a new one is refused, not guessed.
- **The scale** is typed as a number or a fraction (`1/1024`) and shown back as one; a scale left out is 1.
- **One instance** is the settings page's port: a second copy can't listen on it.
- **New token** takes a second click on the page instead of `confirm()`, which would block a page driven over CDP.
- **The status** goes once after the catalogue at every connection, not only on a change, so a board knows HWiNFO's state from the start.
- **The source** is read at the fastest connected board's interval, never below 250 ms, and once a second with none, for the page.
- **Go 1.26 or later** (the spec said 1.24): `golang.org/x/sys` v0.49 requires it, and the owner's Mac has 1.27.2.
- **A LAN port that is taken** keeps the app up with the page and the tray (the review's Important 4); a new port opens at once rather than after a restart, as the spec had it.

---

### Task 1: The settings file, the token and reading ids

**Files:**
- Create: `go.mod`, `go.sum`, `internal/config/config.go`, `internal/config/config_test.go`, `LICENSE` (reflbo's Apache-2.0 text, copied), `NOTICE`, `.gitignore`

**Interfaces:**
- Consumes: nothing; the module starts here.
- Produces (`internal/config`):
  - `type Reading struct { Key, ID, Label, Unit string; Scale float64; Decimals int; Min, Max *float64 }` (JSON `key id label unit scale decimals min max`);
  - `type Config struct { Schema, Port, UIPort int; Token string; Readings []Reading }` (JSON `schema port ui_port token readings`);
  - `Default() Config` (ports 47123 and 47124, a new token, no readings);
  - `Path() (string, error)` (`%APPDATA%\reflbo-stats\config.json`);
  - `NewToken() string`, `Grouped(string) string`, `Ungrouped(string) string`;
  - `Load(path) (Config, error)` (missing: the defaults and an `os.IsNotExist` error);
  - `LoadOrBak(path) (Config, error)`;
  - `Save(path, Config) error` (atomic, the previous file kept as `.bak`);
  - `Validate(Config) error`, whose messages the page shows as they are;
  - `MakeID(label string, taken map[string]bool) string`;
  - the constants `Schema`, `MaxReadings` (32), `MaxIDLen` (15), `MaxLabelLen` (23), `MaxUnitLen` (7).

- [ ] **Step 1: The module.**

```bash
cd ~/reflbo-stats-win
git checkout -b plan/m7
go mod init github.com/Vybo/reflbo-stats-win && go mod edit -go=1.24
mkdir -p cmd/reflbo-stats internal/config internal/hwinfo internal/source internal/server internal/autostart web testdata
cp ~/reflbo/LICENSE LICENSE
```

`NOTICE`:

```text
reflbo-stats
Copyright 2026 Dan Vybiral

This product is licensed under the Apache License, Version 2.0 (see LICENSE).
```


`.gitignore`:

```text
/dist/
*.exe
/testdata/*.local.bin
```


- [ ] **Step 2: Write the failing tests.** The defaults and the token's shape; a save that keeps the last file as `.bak` and leaves no temporary file; a missing file, a broken one that falls back to its `.bak`, and `"readings": null` (a hand edit) read as none; each refusal word for word; ids from labels, accents dropped, cut at 15 and kept unique.

`internal/config/config_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func f(v float64) *float64 { return &v }

func good() Config {
	c := Default()
	c.Readings = []Reading{
		{Key: "0xf0000400:0:0x1000000", ID: "cpu_load", Label: "CPU load", Unit: "%", Scale: 1, Decimals: 0, Min: f(0), Max: f(100)},
		{Key: "0xf0000400:0:0x2000000", ID: "ram_used", Label: "RAM used", Unit: "GB", Scale: 1.0 / 1024, Decimals: 1},
	}
	return c
}

func TestDefaultsAreThePortsAndANewToken(t *testing.T) {
	c := Default()
	if c.Schema != 1 || c.Port != 47123 || c.UIPort != 47124 || len(c.Readings) != 0 {
		t.Fatalf("defaults: %+v", c)
	}
	if !regexp.MustCompile(`^[A-Z2-7]{26}$`).MatchString(c.Token) {
		t.Fatalf("token %q is not 26 base32 characters", c.Token)
	}
	if Default().Token == c.Token {
		t.Fatal("two defaults share a token")
	}
}

func TestATokenShowsInGroupsOfFive(t *testing.T) {
	if got := Grouped("ABCDEFGHIJKLMNOPQRSTUVWXYZ"); got != "ABCDE-FGHIJ-KLMNO-PQRST-UVWXY-Z" {
		t.Fatalf("grouped: %q", got)
	}
	if got := Ungrouped(" abcde-fghij klmno "); got != "ABCDEFGHIJKLMNO" {
		t.Fatalf("ungrouped: %q", got)
	}
}

func TestSaveThenLoadKeepsEverything(t *testing.T) {
	path := filepath.Join(t.TempDir(), "reflbo-stats", "config.json")
	c := good()
	if err := Save(path, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Token != c.Token || len(got.Readings) != 2 || got.Readings[1].Scale != 1.0/1024 || *got.Readings[0].Max != 100 ||
		got.Readings[1].Min != nil {
		t.Fatalf("round trip: %+v", got)
	}
}

func TestASaveKeepsThePreviousFileAsBak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	first := good()
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}
	second := good()
	second.Port = 48000
	if err := Save(path, second); err != nil {
		t.Fatal(err)
	}
	bak, err := Load(path + ".bak")
	if err != nil || bak.Port != 47123 {
		t.Fatalf(".bak: %+v %v", bak, err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".config.json.") {
			t.Fatalf("a temporary file stayed: %s", e.Name())
		}
	}
}

func TestAMissingFileIsTheDefaults(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if !os.IsNotExist(err) || got.Port != 47123 || got.Token == "" {
		t.Fatalf("missing: %+v %v", got, err)
	}
}

func TestABrokenFileFallsBackToItsBak(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := Save(path, good()); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, good()); err != nil { /* the .bak now holds a good file */
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadOrBak(path)
	if err != nil || len(got.Readings) != 2 {
		t.Fatalf("fallback: %+v %v", got, err)
	}
}

func TestValidationNamesWhatIsWrong(t *testing.T) {
	cases := []struct {
		edit func(*Config)
		want string
	}{
		{func(c *Config) { c.Readings[0].ID = "CPU" }, `id "CPU": 1-15 characters of a-z, 0-9 and _`},
		{func(c *Config) { c.Readings[0].ID = "a_very_long_identifier" }, `id "a_very_long_identifier": 1-15 characters of a-z, 0-9 and _`},
		{func(c *Config) { c.Readings[1].ID = "cpu_load" }, `id "cpu_load" is used twice`},
		{func(c *Config) { c.Readings[0].Label = strings.Repeat("x", 24) }, `cpu_load: a label is at most 23 bytes`},
		{func(c *Config) { c.Readings[0].Unit = "percent" + "s" }, `cpu_load: a unit is at most 7 bytes`},
		{func(c *Config) { c.Readings[0].Decimals = 4 }, `cpu_load: decimals are 0-3`},
		{func(c *Config) { c.Readings[0].Scale = 0 }, `cpu_load: the scale must be above 0`},
		{func(c *Config) { c.Readings[0].Min = f(100) }, `cpu_load: min must be below max`},
		{func(c *Config) { c.Port = 80 }, `ports are 1024-65535 and differ`},
		{func(c *Config) { c.UIPort = c.Port }, `ports are 1024-65535 and differ`},
		{func(c *Config) { c.Token = "short" }, `the token is 26 base32 characters`},
		{func(c *Config) {
			for i := 0; i < 31; i++ {
				c.Readings = append(c.Readings, Reading{Key: "k", ID: "r" + string(rune('a'+i%26)) + string(rune('a'+i/26)), Label: "x", Scale: 1})
			}
		}, `at most 32 readings`},
	}
	if err := Validate(good()); err != nil {
		t.Fatalf("a good config: %v", err)
	}
	for _, tc := range cases {
		c := good()
		tc.edit(&c)
		err := Validate(c)
		if err == nil || err.Error() != tc.want {
			t.Errorf("want %q, got %v", tc.want, err)
		}
	}
}

func TestAnIdIsMadeFromTheLabelAndKeptUnique(t *testing.T) {
	taken := map[string]bool{"cpu_package": true}
	cases := map[string]string{
		"CPU Total":                 "cpu_total",
		"CPU Package":               "cpu_package_2",
		"GPU Temperature":           "gpu_temperature",
		"Physical Memory Used":      "physical_memory",
		"Čtení (MB/s)":              "cteni_mb_s",
		"  ":                        "reading",
		"Core 0 T0 Effective Clock": "core_0_t0_effec",
	}
	for label, want := range cases {
		if got := MakeID(label, taken); got != want {
			t.Errorf("%q: want %q, got %q", label, want, got)
		}
	}
}

func TestNullReadingsAreNone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c := good()
	data := `{"schema":1,"port":47123,"ui_port":47124,"token":"` + c.Token + `","readings":null}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil || got.Readings == nil || len(got.Readings) != 0 {
		t.Fatalf("null readings: %#v %v", got.Readings, err)
	}
}
```


- [ ] **Step 3: Run them.**

Run: `go test ./internal/config/`

Expected: the build fails, as nothing is defined yet.

```text
FAIL	github.com/Vybo/reflbo-stats-win/internal/config [build failed]
FAIL
# github.com/Vybo/reflbo-stats-win/internal/config [github.com/Vybo/reflbo-stats-win/internal/config.test]
internal/config/config_test.go:13:13: undefined: Config
internal/config/config_test.go:14:7: undefined: Default
internal/config/config_test.go:15:17: undefined: Reading
internal/config/config_test.go:23:7: undefined: Default
internal/config/config_test.go:30:5: undefined: Default
internal/config/config_test.go:36:12: undefined: Grouped
internal/config/config_test.go:39:12: undefined: Ungrouped
internal/config/config_test.go:47:12: undefined: Save
internal/config/config_test.go:50:14: undefined: Load
internal/config/config_test.go:63:12: undefined: Save
internal/config/config_test.go:63:12: too many errors
```

- [ ] **Step 4: Write the package.** Accents go through a small table (Latin-1 and Latin Extended-A), not `golang.org/x/text`, to keep the dependency list as the spec has it; HWiNFO's labels are ASCII nearly always.

`internal/config/config.go`:

```go
// Package config is reflbo-stats' settings file (spec §9): the picked readings, the token and the ports.
package config

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

const (
	Schema      = 1
	MaxReadings = 32 // reflbo spec §12a.3
	MaxIDLen    = 15
	MaxLabelLen = 23 // bytes
	MaxUnitLen  = 7  // bytes
	tokenLen    = 26 // 128 random bits in base32, unpadded
)

// Reading is one HWiNFO reading the owner picked, as the boards see it (spec §5).
type Reading struct {
	Key      string   `json:"key"` // HWiNFO's sensor id, sensor instance and reading id: "0xf0000400:0:0x1000000"
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Unit     string   `json:"unit"`
	Scale    float64  `json:"scale"`
	Decimals int      `json:"decimals"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
}

type Config struct {
	Schema   int       `json:"schema"`
	Port     int       `json:"port"`
	UIPort   int       `json:"ui_port"`
	Token    string    `json:"token"`
	Readings []Reading `json:"readings"`
}

// Default is a first start's settings, with a new token.
func Default() Config {
	return Config{Schema: Schema, Port: 47123, UIPort: 47124, Token: NewToken(), Readings: []Reading{}}
}

// Path is where the file lives: %APPDATA%\reflbo-stats\config.json on Windows.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "reflbo-stats", "config.json"), nil
}

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewToken is 128 random bits as 26 base32 characters (A-Z, 2-7).
func NewToken() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand doesn't fail on the systems we run on
	}
	return b32.EncodeToString(b[:])
}

// Grouped shows a token in groups of five, as the page does.
func Grouped(token string) string {
	var parts []string
	for len(token) > 5 {
		parts = append(parts, token[:5])
		token = token[5:]
	}
	return strings.Join(append(parts, token), "-")
}

// Ungrouped undoes Grouped and the ways a token gets typed: spaces, dashes, lower case.
func Ungrouped(s string) string {
	return strings.ToUpper(strings.NewReplacer(" ", "", "-", "").Replace(s))
}

// Load reads the file; a missing one gives the defaults with an error os.IsNotExist recognises.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Default(), err
	}
	c := Config{Readings: []Reading{}}
	if err := json.Unmarshal(data, &c); err != nil {
		return Default(), fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	if c.Readings == nil { /* "readings": null, as a hand edit leaves it, is none */
		c.Readings = []Reading{}
	}
	if c.Schema != Schema {
		return Default(), fmt.Errorf("%s: schema %d, not %d", filepath.Base(path), c.Schema, Schema)
	}
	if err := Validate(c); err != nil {
		return Default(), fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return c, nil
}

// LoadOrBak reads the file, or its .bak when the file is missing or broken; the defaults when both are.
func LoadOrBak(path string) (Config, error) {
	c, err := Load(path)
	if err == nil {
		return c, nil
	}
	if bak, berr := Load(path + ".bak"); berr == nil {
		return bak, nil
	}
	return c, err
}

// Save writes the file atomically (a temporary file, then a rename), the previous version kept as .bak.
func Save(path string, c Config) error {
	if err := Validate(c); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // gone after the rename; cleans up after a failure
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, path+".bak"); err != nil {
			return err
		}
	}
	return os.Rename(tmp.Name(), path)
}

var idRule = regexp.MustCompile(`^[a-z0-9_]{1,15}$`)

// Validate names the first thing wrong, in the page's words.
func Validate(c Config) error {
	if c.Port < 1024 || c.Port > 65535 || c.UIPort < 1024 || c.UIPort > 65535 || c.Port == c.UIPort {
		return errors.New("ports are 1024-65535 and differ")
	}
	if len(c.Token) != tokenLen {
		return errors.New("the token is 26 base32 characters")
	}
	if _, err := b32.DecodeString(c.Token); err != nil {
		return errors.New("the token is 26 base32 characters")
	}
	if len(c.Readings) > MaxReadings {
		return fmt.Errorf("at most %d readings", MaxReadings)
	}
	seen := map[string]bool{}
	for _, r := range c.Readings {
		switch {
		case !idRule.MatchString(r.ID):
			return fmt.Errorf("id %q: 1-15 characters of a-z, 0-9 and _", r.ID)
		case seen[r.ID]:
			return fmt.Errorf("id %q is used twice", r.ID)
		case len(r.Label) > MaxLabelLen:
			return fmt.Errorf("%s: a label is at most %d bytes", r.ID, MaxLabelLen)
		case len(r.Unit) > MaxUnitLen:
			return fmt.Errorf("%s: a unit is at most %d bytes", r.ID, MaxUnitLen)
		case r.Decimals < 0 || r.Decimals > 3:
			return fmt.Errorf("%s: decimals are 0-3", r.ID)
		case !(r.Scale > 0) || math.IsInf(r.Scale, 0):
			return fmt.Errorf("%s: the scale must be above 0", r.ID)
		case r.Min != nil && r.Max != nil && !(*r.Min < *r.Max):
			return fmt.Errorf("%s: min must be below max", r.ID)
		}
		seen[r.ID] = true
	}
	return nil
}

// unaccented maps Latin letters with accents (Latin-1 and Latin Extended-A, Czech's among them) to plain ones.
var unaccented = func() map[rune]rune {
	m := map[rune]rune{}
	for plain, accented := range map[rune]string{
		'a': "àáâãäåāăą", 'c': "çćĉċč", 'd': "ďđ", 'e': "èéêëēĕėęě", 'g': "ĝğġģ", 'h': "ĥħ", 'i': "ìíîïĩīĭįı",
		'j': "ĵ", 'k': "ķ", 'l': "ĺļľŀł", 'n': "ñńņňŉ", 'o': "òóôõöøōŏő", 'r': "ŕŗř", 's': "śŝşš", 't': "ţťŧ",
		'u': "ùúûüũūŭůűų", 'w': "ŵ", 'y': "ýÿŷ", 'z': "źżž",
	} {
		for _, r := range accented {
			m[r] = plain
			m[unicode.ToUpper(r)] = unicode.ToUpper(plain)
		}
	}
	return m
}()

// MakeID makes an id from a label: lower case, accents dropped, other runs of characters as one _, at most
// 15 characters, then _2, _3 … until it is free in `taken` (spec §5).
func MakeID(label string, taken map[string]bool) string {
	var b strings.Builder
	under := false
	for _, r := range label {
		if plain, ok := unaccented[r]; ok {
			r = plain
		}
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			if under && b.Len() > 0 {
				b.WriteByte('_')
			}
			under = false
			b.WriteRune(unicode.ToLower(r))
		default:
			under = true
		}
	}
	base := b.String()
	if base == "" {
		base = "reading"
	}
	cut := func(s string, n int) string {
		if len(s) > n {
			s = s[:n]
		}
		return strings.TrimRight(s, "_")
	}
	id := cut(base, MaxIDLen)
	for n := 2; taken[id]; n++ {
		suffix := fmt.Sprintf("_%d", n)
		id = cut(base, MaxIDLen-len(suffix)) + suffix
	}
	return id
}
```


- [ ] **Step 5: Run them again.**

Run: `gofmt -l internal; go vet ./internal/config/ && go test ./internal/config/`

Expected: nothing from `gofmt`, and the package passes.

```text
ok  	github.com/Vybo/reflbo-stats-win/internal/config	(cached)
```

- [ ] **Step 6: Commit.**

`go.mod`:

```text
module github.com/Vybo/reflbo-stats-win

go 1.24
```


```bash
go mod tidy
git add -A
git commit -m "feat(config): the settings file, the token and reading ids (spec §5, §9)"
```

### Task 2: HWiNFO's shared memory

**Files:**
- Create: `internal/hwinfo/hwinfo.go`, `internal/hwinfo/hwinfo_test.go`, `internal/hwinfo/mapping_windows.go`, `internal/hwinfo/mapping_other.go`
- Modify: `go.mod`, `go.sum` (`golang.org/x/sys`)

**Interfaces:**
- Consumes: nothing from Task 1.
- Produces (`internal/hwinfo`):
  - `Parse(b []byte) (*Snapshot, error)`, with `ErrDead` for a mapping HWiNFO closed (signature `DEAD`);
  - `type Snapshot struct { Version, Revision uint32; PollTime int64; PollingPeriodMS uint32; Sensors []Sensor; Readings []Reading }`: the poll time and HWiNFO's polling period, which Task 3's staleness rule needs;
  - `(*Snapshot).Key(Reading) string`, as `0x<sensor id>:<instance>:0x<reading id>`;
  - `type Sensor struct { ID, Instance uint32; Name string }`;
  - `type Reading struct { Type ReadingType; Sensor int; ID uint32; Label, Unit string; Value, Min, Max, Avg float64 }`;
  - the types `TypeNone` … `TypeOther` (HWiNFO's order);
  - `ReadMapping() ([]byte, error)` and `Running() bool` (Windows; elsewhere `ErrNoMapping` and false), with `ErrNoMapping`;
  - the constants `MappingName`, `MutexName`, `SignatureLive`, `SignatureDead`, `HeaderSize` (48).

HWiNFO's layout, packed and little-endian, as its published header `hwisenssm2.h` gives it:
- **Header:** signature, version, revision, the poll time (64-bit), then offset, size and count of the sensor section and of the reading section, then the polling period.
- **A sensor:** id, instance, the original name (128 bytes), the user's name (128); from 392 bytes, the user's name in UTF-8.
- **A reading:**
  - type, sensor index, id;
  - the original label (128), the user's label (128), the unit (16);
  - value, min, max and average as doubles;
  - from 460 bytes, the user's label (128) and the unit (16) in UTF-8.

The element sizes come from the header, so a longer element of a newer HWiNFO still reads.

- [ ] **Step 1: Write the failing tests.** They build mappings in that layout:
  - with and without the UTF-8 fields, and with longer elements;
  - each refusal (too short, not HWiNFO's, an unknown version, elements too small, truncated, a count beyond reason);
  - a closed mapping;
  - a reading of a sensor that isn't there;
  - the polling period, at the header's offset 44.

  The builder lives in the test for now; Task 3 moves it to a package of its own.

`internal/hwinfo/hwinfo_test.go`:

```go
package hwinfo

import (
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"
)

// fixture builds a mapping as HWiNFO lays it out (hwisenssm2.h): the header, then the sensors, then the
// readings, each element `sensorSize` or `readingSize` bytes.
type fsensor struct {
	id, inst         uint32
	orig, user, utf8 string
}

type freading struct {
	typ                  ReadingType
	sensor               uint32
	id                   uint32
	orig, user, unit     string
	utfLabel, utfUnit    string
	value, min, max, avg float64
}

func fixture(sig, version uint32, poll int64, sensorSize, readingSize int, sensors []fsensor, readings []freading) []byte {
	sensorOff := HeaderSize
	readingOff := sensorOff + sensorSize*len(sensors)
	b := make([]byte, readingOff+readingSize*len(readings))
	le := binary.LittleEndian
	le.PutUint32(b[0:], sig)
	le.PutUint32(b[4:], version)
	le.PutUint32(b[8:], 2)
	le.PutUint64(b[12:], uint64(poll))
	le.PutUint32(b[20:], uint32(sensorOff))
	le.PutUint32(b[24:], uint32(sensorSize))
	le.PutUint32(b[28:], uint32(len(sensors)))
	le.PutUint32(b[32:], uint32(readingOff))
	le.PutUint32(b[36:], uint32(readingSize))
	le.PutUint32(b[40:], uint32(len(readings)))
	le.PutUint32(b[44:], 2000)
	for i, s := range sensors {
		e := b[sensorOff+i*sensorSize:]
		le.PutUint32(e[0:], s.id)
		le.PutUint32(e[4:], s.inst)
		copy(e[8:8+127], s.orig)
		copy(e[136:136+127], s.user)
		if sensorSize >= sensorUTFSize {
			copy(e[264:264+127], s.utf8)
		}
	}
	for i, r := range readings {
		e := b[readingOff+i*readingSize:]
		le.PutUint32(e[0:], uint32(r.typ))
		le.PutUint32(e[4:], r.sensor)
		le.PutUint32(e[8:], r.id)
		copy(e[12:12+127], r.orig)
		copy(e[140:140+127], r.user)
		copy(e[268:268+15], r.unit)
		le.PutUint64(e[284:], math.Float64bits(r.value))
		le.PutUint64(e[292:], math.Float64bits(r.min))
		le.PutUint64(e[300:], math.Float64bits(r.max))
		le.PutUint64(e[308:], math.Float64bits(r.avg))
		if readingSize >= readingUTFSize {
			copy(e[316:316+127], r.utfLabel)
			copy(e[444:444+15], r.utfUnit)
		}
	}
	return b
}

func sample(sensorSize, readingSize int) []byte {
	return fixture(SignatureLive, 1, 1760880000, sensorSize, readingSize,
		[]fsensor{
			{id: 0xf0000400, inst: 0, orig: "CPU [#0]: AMD Ryzen 7 7800X3D", utf8: "CPU [#0]: AMD Ryzen 7 7800X3D"},
			{id: 0xe0002000, inst: 1, orig: "GPU [#0]: NVIDIA GeForce RTX 4070", user: "My GPU", utf8: "My GPU"},
		},
		[]freading{
			{typ: TypeUsage, sensor: 0, id: 0x7000000, orig: "Total CPU Usage", unit: "%", utfLabel: "Total CPU Usage", utfUnit: "%", value: 12.5, min: 0.4, max: 98, avg: 10.1},
			{typ: TypeTemp, sensor: 0, id: 0x1000000, orig: "CPU (Tctl/Tdie)", user: "CPU", unit: "\xb0C", utfLabel: "CPU", utfUnit: "°C", value: 54.25},
			{typ: TypeTemp, sensor: 1, id: 0x1000000, orig: "GPU Temperature", unit: "\xb0C", value: 41},
		})
}

func TestTheSensorsAndReadingsAreRead(t *testing.T) {
	s, err := Parse(sample(sensorUTFSize, readingUTFSize))
	if err != nil {
		t.Fatal(err)
	}
	if s.Version != 1 || s.Revision != 2 || s.PollTime != 1760880000 || len(s.Sensors) != 2 || len(s.Readings) != 3 {
		t.Fatalf("snapshot: %+v", s)
	}
	cpu := s.Readings[0]
	if cpu.Type != TypeUsage || cpu.Sensor != 0 || cpu.Label != "Total CPU Usage" || cpu.Unit != "%" || cpu.Value != 12.5 ||
		cpu.Min != 0.4 || cpu.Max != 98 || cpu.Avg != 10.1 {
		t.Fatalf("reading: %+v", cpu)
	}
	if s.Sensors[1].Name != "My GPU" || s.Readings[1].Label != "CPU" || s.Readings[1].Unit != "°C" {
		t.Fatalf("the owner's labels and UTF-8: %+v / %+v", s.Sensors[1], s.Readings[1])
	}
	if got := s.Key(cpu); got != "0xf0000400:0:0x7000000" {
		t.Fatalf("key %q", got)
	}
	if got := s.Key(s.Readings[2]); got != "0xe0002000:1:0x1000000" {
		t.Fatalf("key %q", got)
	}
}

func TestAnOlderLayoutWithoutUTF8ReadsItsANSIText(t *testing.T) {
	s, err := Parse(sample(sensorBaseSize, readingBaseSize))
	if err != nil {
		t.Fatal(err)
	}
	if s.Readings[1].Label != "CPU" || s.Readings[1].Unit != "°C" || s.Readings[2].Label != "GPU Temperature" {
		t.Fatalf("ANSI: %+v %+v", s.Readings[1], s.Readings[2])
	}
	if s.Sensors[1].Name != "My GPU" || s.Sensors[0].Name != "CPU [#0]: AMD Ryzen 7 7800X3D" {
		t.Fatalf("sensor names: %+v", s.Sensors)
	}
}

func TestLongerElementsOfANewerHWiNFOStillRead(t *testing.T) {
	s, err := Parse(sample(sensorUTFSize+64, readingUTFSize+40))
	if err != nil || len(s.Readings) != 3 || s.Readings[2].Value != 41 {
		t.Fatalf("longer elements: %+v %v", s, err)
	}
}

func TestWhatIsNotAMappingIsRefusedWithItsReason(t *testing.T) {
	good := sample(sensorUTFSize, readingUTFSize)
	cases := []struct {
		name string
		b    []byte
		want string
	}{
		{"short", good[:20], "HWiNFO's shared memory is too short (20 bytes)"},
		{"signature", func() []byte { b := append([]byte{}, good...); copy(b, "ABCD"); return b }(), "not HWiNFO's shared memory"},
		{"version", fixture(SignatureLive, 9, 0, sensorUTFSize, readingUTFSize, nil, nil), "HWiNFO's shared memory version 9 is unknown to this app"},
		{"sensor size", fixture(SignatureLive, 1, 0, 100, readingUTFSize, nil, nil), "HWiNFO's sensors are 100 bytes each, less than the 264 this app reads"},
		{"reading size", fixture(SignatureLive, 1, 0, sensorUTFSize, 300, nil, nil), "HWiNFO's readings are 300 bytes each, less than the 316 this app reads"},
		{"truncated", good[:len(good)-1], "HWiNFO's shared memory is truncated"},
	}
	for _, c := range cases {
		_, err := Parse(c.b)
		if err == nil || err.Error() != c.want {
			t.Errorf("%s: want %q, got %v", c.name, c.want, err)
		}
	}
}

func TestAClosedHWiNFOIsDead(t *testing.T) {
	_, err := Parse(fixture(SignatureDead, 1, 0, sensorUTFSize, readingUTFSize, nil, nil))
	if !errors.Is(err, ErrDead) {
		t.Fatalf("want ErrDead, got %v", err)
	}
}

func TestAReadingOfAnUnknownSensorIsLeftOut(t *testing.T) {
	b := fixture(SignatureLive, 1, 0, sensorUTFSize, readingUTFSize, []fsensor{{id: 1, orig: "A"}},
		[]freading{{sensor: 0, id: 1, orig: "kept"}, {sensor: 5, id: 2, orig: "orphan"}})
	s, err := Parse(b)
	if err != nil || len(s.Readings) != 1 || s.Readings[0].Label != "kept" {
		t.Fatalf("orphans: %+v %v", s, err)
	}
}

func TestCountsBeyondReasonAreRefused(t *testing.T) {
	b := fixture(SignatureLive, 1, 0, sensorUTFSize, readingUTFSize, nil, nil)
	binary.LittleEndian.PutUint32(b[40:], 1<<30)
	if _, err := Parse(b); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("a huge count: %v", err)
	}
}

func TestThePollingPeriodIsRead(t *testing.T) {
	s, err := Parse(sample(sensorUTFSize, readingUTFSize))
	if err != nil || s.PollingPeriodMS != 2000 {
		t.Fatalf("polling period: %+v %v", s, err)
	}
}
```


- [ ] **Step 2: Run them.**

Run: `go test ./internal/hwinfo/`

Expected: the build fails.

```text
FAIL	github.com/Vybo/reflbo-stats-win/internal/hwinfo [build failed]
FAIL
# github.com/Vybo/reflbo-stats-win/internal/hwinfo [github.com/Vybo/reflbo-stats-win/internal/hwinfo.test]
internal/hwinfo/hwinfo_test.go:19:23: undefined: ReadingType
internal/hwinfo/hwinfo_test.go:28:15: undefined: HeaderSize
internal/hwinfo/hwinfo_test.go:49:20: undefined: sensorUTFSize
internal/hwinfo/hwinfo_test.go:65:21: undefined: readingUTFSize
internal/hwinfo/hwinfo_test.go:74:17: undefined: SignatureLive
internal/hwinfo/hwinfo_test.go:80:10: undefined: TypeUsage
internal/hwinfo/hwinfo_test.go:81:10: undefined: TypeTemp
internal/hwinfo/hwinfo_test.go:82:10: undefined: TypeTemp
internal/hwinfo/hwinfo_test.go:87:12: undefined: Parse
internal/hwinfo/hwinfo_test.go:87:25: undefined: sensorUTFSize
internal/hwinfo/hwinfo_test.go:87:25: too many errors
```

- [ ] **Step 3: Write the parser.** Text is the UTF-8 user text when the layout has it and it's set, else the user's ANSI text, else HWiNFO's own. ANSI that isn't UTF-8 reads as Latin-1, which covers the units' `°`.

`internal/hwinfo/hwinfo.go`:

```go
// Package hwinfo reads HWiNFO's shared memory (spec §4): the layout of HWiNFO's published header,
// hwisenssm2.h, packed, little-endian. Parse is pure Go; reading the mapping itself is Windows-only.
package hwinfo

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"unicode/utf8"
)

const (
	MappingName = `Global\HWiNFO_SENS_SM2`
	MutexName   = `Global\HWiNFO_SM2_MUTEX`

	SignatureLive = 0x53695748 // "HWiS"
	SignatureDead = 0x44414544 // "DEAD": HWiNFO closed it

	HeaderSize = 48

	sensorBaseSize  = 264 // id, instance, the original and the user's name
	sensorUTFSize   = 392 // and the user's name in UTF-8
	readingBaseSize = 316 // type, sensor, id, two labels, unit, value, min, max, average
	readingUTFSize  = 460 // and the user's label and the unit in UTF-8

	maxSensors  = 4096
	maxReadings = 65536
)

// The versions of the layout this app knows; a new one is refused rather than misread.
var knownVersions = map[uint32]bool{1: true, 2: true}

type ReadingType uint32

const (
	TypeNone ReadingType = iota
	TypeTemp
	TypeVolt
	TypeFan
	TypeCurrent
	TypePower
	TypeClock
	TypeUsage
	TypeOther
)

type Sensor struct {
	ID       uint32
	Instance uint32
	Name     string // the owner's name in HWiNFO if set, else HWiNFO's
}

type Reading struct {
	Type   ReadingType
	Sensor int // index into Snapshot.Sensors
	ID     uint32
	Label  string // the owner's label in HWiNFO if set, else HWiNFO's
	Unit   string
	Value  float64
	Min    float64
	Max    float64
	Avg    float64
}

type Snapshot struct {
	Version  uint32
	Revision uint32
	PollTime int64 // HWiNFO's last poll, Unix seconds: stale when it stops moving
	// HWiNFO's polling period, ms (its Settings); 0 where the header doesn't say
	PollingPeriodMS uint32
	Sensors         []Sensor
	Readings        []Reading
}

// ErrDead is a mapping HWiNFO closed on its way out.
var ErrDead = errors.New("HWiNFO has closed its shared memory")

// Key names a reading across HWiNFO's restarts: its sensor's id and instance and its own id.
func (s *Snapshot) Key(r Reading) string {
	sn := s.Sensors[r.Sensor]
	return fmt.Sprintf("0x%x:%d:0x%x", sn.ID, sn.Instance, r.ID)
}

// Parse reads a copy of the mapping.
func Parse(b []byte) (*Snapshot, error) {
	if len(b) < HeaderSize {
		return nil, fmt.Errorf("HWiNFO's shared memory is too short (%d bytes)", len(b))
	}
	le := binary.LittleEndian
	switch le.Uint32(b[0:]) {
	case SignatureLive:
	case SignatureDead:
		return nil, ErrDead
	default:
		return nil, errors.New("not HWiNFO's shared memory")
	}
	s := &Snapshot{Version: le.Uint32(b[4:]), Revision: le.Uint32(b[8:]), PollTime: int64(le.Uint64(b[12:])),
		PollingPeriodMS: le.Uint32(b[44:])}
	if !knownVersions[s.Version] {
		return nil, fmt.Errorf("HWiNFO's shared memory version %d is unknown to this app", s.Version)
	}
	sOff, sSize, sCount := le.Uint32(b[20:]), le.Uint32(b[24:]), le.Uint32(b[28:])
	rOff, rSize, rCount := le.Uint32(b[32:]), le.Uint32(b[36:]), le.Uint32(b[40:])
	if sSize < sensorBaseSize {
		return nil, fmt.Errorf("HWiNFO's sensors are %d bytes each, less than the %d this app reads", sSize, sensorBaseSize)
	}
	if rSize < readingBaseSize {
		return nil, fmt.Errorf("HWiNFO's readings are %d bytes each, less than the %d this app reads", rSize, readingBaseSize)
	}
	if !fits(len(b), sOff, sSize, sCount, maxSensors) || !fits(len(b), rOff, rSize, rCount, maxReadings) {
		return nil, errors.New("HWiNFO's shared memory is truncated")
	}
	for i := uint32(0); i < sCount; i++ {
		e := b[sOff+i*sSize : sOff+(i+1)*sSize]
		name := pick(e, sSize >= sensorUTFSize, 264, 128, 136, 8)
		s.Sensors = append(s.Sensors, Sensor{ID: le.Uint32(e[0:]), Instance: le.Uint32(e[4:]), Name: name})
	}
	for i := uint32(0); i < rCount; i++ {
		e := b[rOff+i*rSize : rOff+(i+1)*rSize]
		sensor := le.Uint32(e[4:])
		if sensor >= sCount {
			continue // a reading of a sensor that isn't there
		}
		utf := rSize >= readingUTFSize
		unit := cstring(e[268:284])
		if utf {
			if u := cstring(e[444:460]); u != "" {
				unit = u
			}
		}
		s.Readings = append(s.Readings, Reading{
			Type:   ReadingType(le.Uint32(e[0:])),
			Sensor: int(sensor),
			ID:     le.Uint32(e[8:]),
			Label:  pick(e, utf, 316, 128, 140, 12),
			Unit:   unit,
			Value:  math.Float64frombits(le.Uint64(e[284:])),
			Min:    math.Float64frombits(le.Uint64(e[292:])),
			Max:    math.Float64frombits(le.Uint64(e[300:])),
			Avg:    math.Float64frombits(le.Uint64(e[308:])),
		})
	}
	return s, nil
}

// fits: count elements of size bytes from off lie within n bytes, and count is within reason.
func fits(n int, off, size, count, limit uint32) bool {
	return count <= limit && uint64(off)+uint64(size)*uint64(count) <= uint64(n)
}

// pick: the UTF-8 user text when the layout has it and it is set, else the user's ANSI text, else the original.
func pick(e []byte, utf bool, utfAt, length, userAt, origAt int) string {
	if utf {
		if t := cstring(e[utfAt : utfAt+length]); t != "" {
			return t
		}
	}
	if t := cstring(e[userAt : userAt+length]); t != "" {
		return t
	}
	return cstring(e[origAt : origAt+length])
}

// cstring: the text up to its NUL; ANSI that isn't UTF-8 read as Latin-1, which covers HWiNFO's units (°).
func cstring(field []byte) string {
	n := 0
	for n < len(field) && field[n] != 0 {
		n++
	}
	raw := field[:n]
	if utf8.Valid(raw) {
		return string(raw)
	}
	runes := make([]rune, len(raw))
	for i, c := range raw {
		runes[i] = rune(c)
	}
	return string(runes)
}
```


- [ ] **Step 4: Run them again.**

Run: `gofmt -l internal; go vet ./internal/hwinfo/ && go test ./internal/hwinfo/`

Expected: the package passes.

```text
ok  	github.com/Vybo/reflbo-stats-win/internal/hwinfo	(cached)
```

- [ ] **Step 5: The mapping on Windows, and its stand-in elsewhere.**

```bash
go get golang.org/x/sys@v0.49.0
```

- `golang.org/x/sys/windows` has no `OpenFileMapping`, so `OpenFileMappingW` comes from kernel32.
- The copy is made under HWiNFO's mutex when it has one (50 ms at most; an abandoned mutex is ours all the same). A Windows mutex belongs to the thread that takes it, so the goroutine stays locked to its OS thread until the release (AGENTS gotcha 5): otherwise Go could move it, the release would fail on the other thread, and the mutex would stay taken. The handle is closed only after the release, as deferred calls run last-in, first-out.
- `go vet` flags turning a `uintptr` into a pointer, so the view becomes a slice through a slice header (AGENTS gotcha 2).
- `Running` looks for an `hwinfo*.exe` process, to tell "shm off" from "not running".

`internal/hwinfo/mapping_windows.go`:

```go
//go:build windows

package hwinfo

import (
	"errors"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procOpenFileMapping = windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenFileMappingW")

const fileMapRead = 0x0004 // FILE_MAP_READ

// ErrNoMapping: HWiNFO isn't running, or runs with Shared Memory Support off (Running tells them apart).
var ErrNoMapping = errors.New("HWiNFO's shared memory isn't there")

// ReadMapping copies the whole mapping, under HWiNFO's mutex when it has one, so a poll in progress isn't
// read half-written.
func ReadMapping() ([]byte, error) {
	// A Windows mutex belongs to the thread that takes it: the goroutine stays on one OS thread until it lets go,
	// or ReleaseMutex would fail on another one and the mutex stay taken.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	name, _ := windows.UTF16PtrFromString(MappingName)
	h, _, callErr := procOpenFileMapping.Call(fileMapRead, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		if errors.Is(callErr, windows.ERROR_FILE_NOT_FOUND) {
			return nil, ErrNoMapping
		}
		return nil, callErr
	}
	defer windows.CloseHandle(windows.Handle(h))
	addr, err := windows.MapViewOfFile(windows.Handle(h), fileMapRead, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	defer windows.UnmapViewOfFile(addr)
	var info windows.MemoryBasicInformation
	if err := windows.VirtualQuery(addr, &info, unsafe.Sizeof(info)); err != nil {
		return nil, err
	}
	if mutexName, err := windows.UTF16PtrFromString(MutexName); err == nil {
		if m, err := windows.OpenMutex(windows.SYNCHRONIZE, false, mutexName); err == nil {
			defer windows.CloseHandle(m) // runs last
			ev, _ := windows.WaitForSingleObject(m, 50)
			if ev == windows.WAIT_OBJECT_0 || ev == windows.WAIT_ABANDONED { // abandoned: ours now all the same
				defer windows.ReleaseMutex(m) // runs before the close, after the copy
			}
		}
	}
	var view []byte // the mapped view as a slice, without converting the uintptr itself (go vet's unsafeptr)
	hdr := (*struct {
		data     uintptr
		len, cap int
	})(unsafe.Pointer(&view))
	hdr.data, hdr.len, hdr.cap = addr, int(info.RegionSize), int(info.RegionSize)
	return append([]byte(nil), view...), nil
}

// Running: an HWiNFO process is there (HWiNFO64.exe, HWiNFO32.exe or HWiNFO.exe).
func Running() bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
		if strings.HasPrefix(name, "hwinfo") && strings.HasSuffix(name, ".exe") {
			return true
		}
	}
	return false
}
```


`internal/hwinfo/mapping_other.go`:

```go
//go:build !windows

package hwinfo

import "errors"

// ErrNoMapping: there is no HWiNFO outside Windows; the demo source stands in (spec §3).
var ErrNoMapping = errors.New("HWiNFO's shared memory isn't there")

func ReadMapping() ([]byte, error) { return nil, ErrNoMapping }

func Running() bool { return false }
```


- [ ] **Step 6: Vet for both systems, and commit.**

Run: `go vet ./... && GOOS=windows GOARCH=amd64 go vet ./... && echo vet-ok && go test ./...`

Expected: `vet-ok` and both packages pass.

```text
vet-ok
ok  	github.com/Vybo/reflbo-stats-win/internal/config	(cached)
ok  	github.com/Vybo/reflbo-stats-win/internal/hwinfo	(cached)
```

`go.mod`:

```diff
--- a/go.mod
+++ b/go.mod
@@ -1,3 +1,5 @@
 module github.com/Vybo/reflbo-stats-win
 
-go 1.24
+go 1.26.0
+
+require golang.org/x/sys v0.49.0
```


```bash
go mod tidy
git add -A
git commit -m "feat(hwinfo): HWiNFO's shared memory, read and parsed (spec §4)"
```

### Task 3: The sources, the catalogue and the values

**Files:**
- Create: `internal/hwinfo/hwinfotest/fixture.go`, `internal/source/source.go`, `internal/source/demo.go`, `internal/source/source_test.go`
- Modify: `internal/hwinfo/hwinfo_test.go` (its builder moves to `hwinfotest`)

**Interfaces:**
- Consumes: `hwinfo.Parse`, `ReadMapping`, `Running`, `ErrNoMapping`, `ErrDead`, `(*Snapshot).Key` (Task 2); `config.Reading` (Task 1).
- Produces:
  - `internal/hwinfo/hwinfotest`: `Build(sig, version uint32, poll int64, sensorSize, readingSize int, sensors []Sensor, readings []Reading) []byte`, with the layout's sizes as constants.
  - `internal/source`:
    - `type Status string`: `OK` "ok", `NotRunning` "not running", `ShmOff` "shm off", `Stale` "stale";
    - `type Item struct { Key, Group, Label, Unit, Type string; Value, Min, Max *float64 }`: `nil` for a number that isn't finite, as JSON can't carry NaN or infinity;
    - `type Sample struct { Status Status; Detail string; Items []Item }`;
    - `type Source interface { Sample() Sample }`;
    - `NewHWiNFO() *HWiNFO`, stale after HWiNFO's poll time stands still for three of its polling periods, never under 5 s;
    - `NewDemo(now func() time.Time) *Demo`;
    - `type Entry struct { ID, Label, Unit string; Decimals int; Min, Max *float64 }`;
    - `Catalog([]config.Reading) []Entry`;
    - `Values([]config.Reading, Sample) map[string]*float64`: each scaled and rounded, `nil` when missing, not "ok", or not finite after its scale.

- [ ] **Step 1: Move the mapping builder into a package of its own**, so the source's tests build mappings too, and point Task 2's tests at it.

`internal/hwinfo/hwinfotest/fixture.go`:

```go
// Package hwinfotest builds HWiNFO mappings for tests, as HWiNFO lays them out (hwisenssm2.h).
package hwinfotest

import (
	"encoding/binary"
	"math"
)

const (
	HeaderSize      = 48
	SensorBaseSize  = 264
	SensorUTFSize   = 392
	ReadingBaseSize = 316
	ReadingUTFSize  = 460
	SignatureLive   = 0x53695748
	SignatureDead   = 0x44414544
)

type Sensor struct {
	ID, Inst         uint32
	Orig, User, UTF8 string
}

type Reading struct {
	Type              uint32
	Sensor            uint32
	ID                uint32
	Orig, User, Unit  string
	UTFLabel, UTFUnit string
	Value, Min, Max   float64
	Avg               float64
}

// Build lays out the header, then the sensors, then the readings, each element sensorSize or readingSize bytes.
func Build(sig, version uint32, poll int64, sensorSize, readingSize int, sensors []Sensor, readings []Reading) []byte {
	sensorOff := HeaderSize
	readingOff := sensorOff + sensorSize*len(sensors)
	b := make([]byte, readingOff+readingSize*len(readings))
	le := binary.LittleEndian
	le.PutUint32(b[0:], sig)
	le.PutUint32(b[4:], version)
	le.PutUint32(b[8:], 2)
	le.PutUint64(b[12:], uint64(poll))
	le.PutUint32(b[20:], uint32(sensorOff))
	le.PutUint32(b[24:], uint32(sensorSize))
	le.PutUint32(b[28:], uint32(len(sensors)))
	le.PutUint32(b[32:], uint32(readingOff))
	le.PutUint32(b[36:], uint32(readingSize))
	le.PutUint32(b[40:], uint32(len(readings)))
	le.PutUint32(b[44:], 2000)
	for i, s := range sensors {
		e := b[sensorOff+i*sensorSize:]
		le.PutUint32(e[0:], s.ID)
		le.PutUint32(e[4:], s.Inst)
		copy(e[8:8+127], s.Orig)
		copy(e[136:136+127], s.User)
		if sensorSize >= SensorUTFSize {
			copy(e[264:264+127], s.UTF8)
		}
	}
	for i, r := range readings {
		e := b[readingOff+i*readingSize:]
		le.PutUint32(e[0:], r.Type)
		le.PutUint32(e[4:], r.Sensor)
		le.PutUint32(e[8:], r.ID)
		copy(e[12:12+127], r.Orig)
		copy(e[140:140+127], r.User)
		copy(e[268:268+15], r.Unit)
		le.PutUint64(e[284:], math.Float64bits(r.Value))
		le.PutUint64(e[292:], math.Float64bits(r.Min))
		le.PutUint64(e[300:], math.Float64bits(r.Max))
		le.PutUint64(e[308:], math.Float64bits(r.Avg))
		if readingSize >= ReadingUTFSize {
			copy(e[316:316+127], r.UTFLabel)
			copy(e[444:444+15], r.UTFUnit)
		}
	}
	return b
}
```


`internal/hwinfo/hwinfo_test.go`:

```diff
--- a/internal/hwinfo/hwinfo_test.go
+++ b/internal/hwinfo/hwinfo_test.go
@@ -3,83 +3,29 @@ package hwinfo
 import (
 	"encoding/binary"
 	"errors"
-	"math"
 	"strings"
 	"testing"
-)
 
-// fixture builds a mapping as HWiNFO lays it out (hwisenssm2.h): the header, then the sensors, then the
-// readings, each element `sensorSize` or `readingSize` bytes.
-type fsensor struct {
-	id, inst         uint32
-	orig, user, utf8 string
-}
+	"github.com/Vybo/reflbo-stats-win/internal/hwinfo/hwinfotest"
+)
 
-type freading struct {
-	typ                  ReadingType
-	sensor               uint32
-	id                   uint32
-	orig, user, unit     string
-	utfLabel, utfUnit    string
-	value, min, max, avg float64
-}
+type fsensor = hwinfotest.Sensor
+type freading = hwinfotest.Reading
 
 func fixture(sig, version uint32, poll int64, sensorSize, readingSize int, sensors []fsensor, readings []freading) []byte {
-	sensorOff := HeaderSize
-	readingOff := sensorOff + sensorSize*len(sensors)
-	b := make([]byte, readingOff+readingSize*len(readings))
-	le := binary.LittleEndian
-	le.PutUint32(b[0:], sig)
-	le.PutUint32(b[4:], version)
-	le.PutUint32(b[8:], 2)
-	le.PutUint64(b[12:], uint64(poll))
-	le.PutUint32(b[20:], uint32(sensorOff))
-	le.PutUint32(b[24:], uint32(sensorSize))
-	le.PutUint32(b[28:], uint32(len(sensors)))
-	le.PutUint32(b[32:], uint32(readingOff))
-	le.PutUint32(b[36:], uint32(readingSize))
-	le.PutUint32(b[40:], uint32(len(readings)))
-	le.PutUint32(b[44:], 2000)
-	for i, s := range sensors {
-		e := b[sensorOff+i*sensorSize:]
-		le.PutUint32(e[0:], s.id)
-		le.PutUint32(e[4:], s.inst)
-		copy(e[8:8+127], s.orig)
-		copy(e[136:136+127], s.user)
-		if sensorSize >= sensorUTFSize {
-			copy(e[264:264+127], s.utf8)
-		}
-	}
-	for i, r := range readings {
-		e := b[readingOff+i*readingSize:]
-		le.PutUint32(e[0:], uint32(r.typ))
-		le.PutUint32(e[4:], r.sensor)
-		le.PutUint32(e[8:], r.id)
-		copy(e[12:12+127], r.orig)
-		copy(e[140:140+127], r.user)
-		copy(e[268:268+15], r.unit)
-		le.PutUint64(e[284:], math.Float64bits(r.value))
-		le.PutUint64(e[292:], math.Float64bits(r.min))
-		le.PutUint64(e[300:], math.Float64bits(r.max))
-		le.PutUint64(e[308:], math.Float64bits(r.avg))
-		if readingSize >= readingUTFSize {
-			copy(e[316:316+127], r.utfLabel)
-			copy(e[444:444+15], r.utfUnit)
-		}
-	}
-	return b
+	return hwinfotest.Build(sig, version, poll, sensorSize, readingSize, sensors, readings)
 }
 
 func sample(sensorSize, readingSize int) []byte {
 	return fixture(SignatureLive, 1, 1760880000, sensorSize, readingSize,
 		[]fsensor{
-			{id: 0xf0000400, inst: 0, orig: "CPU [#0]: AMD Ryzen 7 7800X3D", utf8: "CPU [#0]: AMD Ryzen 7 7800X3D"},
-			{id: 0xe0002000, inst: 1, orig: "GPU [#0]: NVIDIA GeForce RTX 4070", user: "My GPU", utf8: "My GPU"},
+			{ID: 0xf0000400, Inst: 0, Orig: "CPU [#0]: AMD Ryzen 7 7800X3D", UTF8: "CPU [#0]: AMD Ryzen 7 7800X3D"},
+			{ID: 0xe0002000, Inst: 1, Orig: "GPU [#0]: NVIDIA GeForce RTX 4070", User: "My GPU", UTF8: "My GPU"},
 		},
 		[]freading{
-			{typ: TypeUsage, sensor: 0, id: 0x7000000, orig: "Total CPU Usage", unit: "%", utfLabel: "Total CPU Usage", utfUnit: "%", value: 12.5, min: 0.4, max: 98, avg: 10.1},
-			{typ: TypeTemp, sensor: 0, id: 0x1000000, orig: "CPU (Tctl/Tdie)", user: "CPU", unit: "\xb0C", utfLabel: "CPU", utfUnit: "°C", value: 54.25},
-			{typ: TypeTemp, sensor: 1, id: 0x1000000, orig: "GPU Temperature", unit: "\xb0C", value: 41},
+			{Type: uint32(TypeUsage), Sensor: 0, ID: 0x7000000, Orig: "Total CPU Usage", Unit: "%", UTFLabel: "Total CPU Usage", UTFUnit: "%", Value: 12.5, Min: 0.4, Max: 98, Avg: 10.1},
+			{Type: uint32(TypeTemp), Sensor: 0, ID: 0x1000000, Orig: "CPU (Tctl/Tdie)", User: "CPU", Unit: "\xb0C", UTFLabel: "CPU", UTFUnit: "°C", Value: 54.25},
+			{Type: uint32(TypeTemp), Sensor: 1, ID: 0x1000000, Orig: "GPU Temperature", Unit: "\xb0C", Value: 41},
 		})
 }
 
@@ -157,8 +103,8 @@ func TestAClosedHWiNFOIsDead(t *testing.T) {
 }
 
 func TestAReadingOfAnUnknownSensorIsLeftOut(t *testing.T) {
-	b := fixture(SignatureLive, 1, 0, sensorUTFSize, readingUTFSize, []fsensor{{id: 1, orig: "A"}},
-		[]freading{{sensor: 0, id: 1, orig: "kept"}, {sensor: 5, id: 2, orig: "orphan"}})
+	b := fixture(SignatureLive, 1, 0, sensorUTFSize, readingUTFSize, []fsensor{{ID: 1, Orig: "A"}},
+		[]freading{{Sensor: 0, ID: 1, Orig: "kept"}, {Sensor: 5, ID: 2, Orig: "orphan"}})
 	s, err := Parse(b)
 	if err != nil || len(s.Readings) != 1 || s.Readings[0].Label != "kept" {
 		t.Fatalf("orphans: %+v %v", s, err)
```


Run: `go test ./internal/hwinfo/...`

Expected: Task 2's tests pass on the moved builder.

```text
ok  	github.com/Vybo/reflbo-stats-win/internal/hwinfo	(cached)
?   	github.com/Vybo/reflbo-stats-win/internal/hwinfo/hwinfotest	[no test files]
```

- [ ] **Step 2: Write the failing tests.**
  - HWiNFO's readings listed with their sensor and type.
  - Its states: no mapping without HWiNFO is "not running", with HWiNFO "shm off"; a closed mapping; an unknown layout and an access refusal both read as "not running" with the reason.
  - Stale after 5 s of a still poll time at a 1 s period, ok again when it moves; after three periods at a 10 s one.
  - A NaN or infinity from HWiNFO, and a value a scale overflows, as none, so the page's state and the readings still encode (AGENTS gotcha 6).
  - The catalogue in order.
  - Values scaled, rounded and `null`.
  - The demo moving, in range and repeatable.

`internal/source/source_test.go`:

```go
package source

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Vybo/reflbo-stats-win/internal/config"
	"github.com/Vybo/reflbo-stats-win/internal/hwinfo"
	"github.com/Vybo/reflbo-stats-win/internal/hwinfo/hwinfotest"
)

func mapping(poll int64, cpu float64) []byte {
	return hwinfotest.Build(hwinfotest.SignatureLive, 1, poll, hwinfotest.SensorUTFSize, hwinfotest.ReadingUTFSize,
		[]hwinfotest.Sensor{{ID: 0xf0000400, Orig: "CPU [#0]: AMD Ryzen 7 7800X3D", UTF8: "CPU [#0]: AMD Ryzen 7 7800X3D"},
			{ID: 0xf0000100, Orig: "System: ASUS", UTF8: "System: ASUS"}},
		[]hwinfotest.Reading{
			{Type: uint32(hwinfo.TypeUsage), Sensor: 0, ID: 0x7000000, Orig: "Total CPU Usage", UTFLabel: "Total CPU Usage", UTFUnit: "%", Value: cpu, Min: 1, Max: 99},
			{Type: uint32(hwinfo.TypeTemp), Sensor: 0, ID: 0x1000000, Orig: "CPU (Tctl/Tdie)", UTFLabel: "CPU (Tctl/Tdie)", UTFUnit: "°C", Value: 54.26},
			{Type: uint32(hwinfo.TypeOther), Sensor: 1, ID: 0x2000000, Orig: "Physical Memory Used", UTFLabel: "Physical Memory Used", UTFUnit: "MB", Value: 21811},
		})
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newTestHWiNFO(read func() ([]byte, error), running bool, c *clock) *HWiNFO {
	h := NewHWiNFO()
	h.read = read
	h.running = func() bool { return running }
	h.now = c.now
	return h
}

func TestHWiNFOsReadingsAreListedWithTheirSensor(t *testing.T) {
	c := &clock{time.Unix(1760880000, 0)}
	h := newTestHWiNFO(func() ([]byte, error) { return mapping(1760880000, 12.5), nil }, true, c)
	s := h.Sample()
	if s.Status != OK || s.Detail != "" || len(s.Items) != 3 {
		t.Fatalf("sample: %+v", s)
	}
	it := s.Items[0]
	if it.Key != "0xf0000400:0:0x7000000" || it.Group != "CPU [#0]: AMD Ryzen 7 7800X3D" || it.Label != "Total CPU Usage" ||
		it.Unit != "%" || *it.Value != 12.5 || it.Type != "usage" {
		t.Fatalf("item: %+v", it)
	}
	if s.Items[1].Type != "temperature" || s.Items[2].Type != "other" {
		t.Fatalf("types: %+v", s.Items)
	}
}

func TestHWiNFOsStates(t *testing.T) {
	c := &clock{time.Unix(1760880000, 0)}
	cases := []struct {
		name    string
		read    func() ([]byte, error)
		running bool
		want    Status
		detail  string
	}{
		{"no HWiNFO", func() ([]byte, error) { return nil, hwinfo.ErrNoMapping }, false, NotRunning, ""},
		{"no shared memory", func() ([]byte, error) { return nil, hwinfo.ErrNoMapping }, true, ShmOff, ""},
		{"closed", func() ([]byte, error) { return nil, hwinfo.ErrDead }, false, NotRunning, ""},
		{"unknown layout", func() ([]byte, error) {
			return hwinfotest.Build(hwinfotest.SignatureLive, 9, 0, hwinfotest.SensorUTFSize, hwinfotest.ReadingUTFSize, nil, nil), nil
		}, true, NotRunning, "HWiNFO's shared memory version 9 is unknown to this app"},
		{"refused", func() ([]byte, error) { return nil, errors.New("Access is denied.") }, true, NotRunning, "Access is denied."},
	}
	for _, tc := range cases {
		s := newTestHWiNFO(tc.read, tc.running, c).Sample()
		if s.Status != tc.want || s.Detail != tc.detail || len(s.Items) != 0 {
			t.Errorf("%s: %+v", tc.name, s)
		}
	}
}

func TestHWiNFOIsStaleWhenItsPollTimeStopsFor5s(t *testing.T) {
	c := &clock{time.Unix(1760880000, 0)}
	poll := int64(1760880000)
	h := newTestHWiNFO(func() ([]byte, error) {
		b := mapping(poll, 10)
		binary.LittleEndian.PutUint32(b[44:], 1000) // HWiNFO polls every second: 5 s at least, not 3
		return b, nil
	}, true, c)
	if s := h.Sample(); s.Status != OK {
		t.Fatalf("first: %v", s.Status)
	}
	c.t = c.t.Add(4900 * time.Millisecond)
	if s := h.Sample(); s.Status != OK {
		t.Fatalf("4.9 s: %v", s.Status)
	}
	c.t = c.t.Add(200 * time.Millisecond)
	if s := h.Sample(); s.Status != Stale || len(s.Items) != 3 {
		t.Fatalf("5.1 s: %+v", s)
	}
	poll++ // HWiNFO polls again
	if s := h.Sample(); s.Status != OK {
		t.Fatalf("moving again: %v", s.Status)
	}
}

func readings() []config.Reading {
	f := func(v float64) *float64 { return &v }
	return []config.Reading{
		{Key: "0xf0000400:0:0x7000000", ID: "cpu_load", Label: "CPU load", Unit: "%", Scale: 1, Decimals: 0, Min: f(0), Max: f(100)},
		{Key: "0xf0000400:0:0x1000000", ID: "cpu_temp", Label: "CPU", Unit: "°C", Scale: 1, Decimals: 1},
		{Key: "0xf0000100:0:0x2000000", ID: "ram_used", Label: "RAM", Unit: "GB", Scale: 1.0 / 1024, Decimals: 1},
		{Key: "0xdead:0:0x1", ID: "gone", Label: "A removed drive", Unit: "%", Scale: 1},
	}
}

func TestTheCatalogueIsThePickedReadingsInOrder(t *testing.T) {
	cat := Catalog(readings())
	if len(cat) != 4 || cat[0].ID != "cpu_load" || *cat[0].Max != 100 || cat[2].Unit != "GB" || cat[2].Decimals != 1 ||
		cat[1].Min != nil {
		t.Fatalf("catalogue: %+v", cat)
	}
}

func TestValuesAreScaledRoundedAndNullWhenMissing(t *testing.T) {
	c := &clock{time.Unix(1760880000, 0)}
	s := newTestHWiNFO(func() ([]byte, error) { return mapping(1760880000, 12.46), nil }, true, c).Sample()
	v := Values(readings(), s)
	if *v["cpu_load"] != 12 || *v["cpu_temp"] != 54.3 || *v["ram_used"] != 21.3 || v["gone"] != nil {
		t.Fatalf("values: %v %v %v %v", *v["cpu_load"], *v["cpu_temp"], *v["ram_used"], v["gone"])
	}
	s.Status = Stale
	for id, x := range Values(readings(), s) {
		if x != nil {
			t.Fatalf("%s should be null while HWiNFO is stale", id)
		}
	}
}

func TestTheDemoMovesAndStaysInRange(t *testing.T) {
	c := &clock{time.Unix(1760880000, 0)}
	d := NewDemo(c.now)
	first := d.Sample()
	if first.Status != OK || len(first.Items) < 8 {
		t.Fatalf("demo: %+v", first)
	}
	keys := map[string]bool{}
	for _, it := range first.Items {
		keys[it.Key] = true
		if it.Unit == "%" && (*it.Value < 0 || *it.Value > 100) {
			t.Fatalf("%s out of range: %v", it.Label, it.Value)
		}
	}
	c.t = c.t.Add(3 * time.Second)
	second := d.Sample()
	moved := false
	for i, it := range second.Items {
		if !keys[it.Key] {
			t.Fatalf("key %s changed", it.Key)
		}
		moved = moved || *it.Value != *first.Items[i].Value
	}
	if !moved {
		t.Fatal("nothing moved in 3 s")
	}
	if a, b := NewDemo(c.now).Sample(), NewDemo(c.now).Sample(); *a.Items[0].Value != *b.Items[0].Value {
		t.Fatal("the demo isn't repeatable for a time")
	}
}

func TestANonFiniteNumberIsNone(t *testing.T) {
	c := &clock{time.Unix(1760880000, 0)}
	b := hwinfotest.Build(hwinfotest.SignatureLive, 1, 1760880000, hwinfotest.SensorUTFSize, hwinfotest.ReadingUTFSize,
		[]hwinfotest.Sensor{{ID: 1, Orig: "CPU"}},
		[]hwinfotest.Reading{{ID: 1, Orig: "Broken", Value: math.NaN(), Min: math.Inf(-1), Max: 3}, {ID: 2, Orig: "Clock", Value: 4300}})
	s := newTestHWiNFO(func() ([]byte, error) { return b, nil }, true, c).Sample()
	if s.Items[0].Value != nil || s.Items[0].Min != nil || *s.Items[0].Max != 3 {
		t.Fatalf("non-finite values: %+v", s.Items[0])
	}
	if _, err := json.Marshal(s); err != nil { /* the page's state still encodes */
		t.Fatal(err)
	}
	picked := []config.Reading{{Key: "0x1:0:0x1", ID: "broken", Scale: 1}, {Key: "0x1:0:0x2", ID: "huge", Scale: 1e306, Decimals: 3}}
	v := Values(picked, s)
	if v["broken"] != nil || v["huge"] != nil {
		t.Fatalf("values: %v %v", v["broken"], v["huge"])
	}
	if _, err := json.Marshal(v); err != nil { /* and so does the readings message */
		t.Fatal(err)
	}
}

func TestStaleWaitsThreeOfHWiNFOsPollingPeriods(t *testing.T) {
	c := &clock{time.Unix(1760880000, 0)}
	b := mapping(1760880000, 10)
	binary.LittleEndian.PutUint32(b[44:], 10000) // HWiNFO polls every 10 s
	h := newTestHWiNFO(func() ([]byte, error) { return b, nil }, true, c)
	h.Sample()
	c.t = c.t.Add(29 * time.Second)
	if s := h.Sample(); s.Status != OK {
		t.Fatalf("29 s at a 10 s period: %v", s.Status)
	}
	c.t = c.t.Add(2 * time.Second)
	if s := h.Sample(); s.Status != Stale {
		t.Fatalf("31 s at a 10 s period: %v", s.Status)
	}
}
```


- [ ] **Step 3: Run them.**

Run: `go test ./internal/source/`

Expected: the build fails.

```text
FAIL	github.com/Vybo/reflbo-stats-win/internal/source [setup failed]
FAIL
# github.com/Vybo/reflbo-stats-win/internal/source
internal/source/source_test.go:13:2: no required module provides package github.com/Vybo/reflbo-stats-win/internal/hwinfo/hwinfotest; to add it:
	go get github.com/Vybo/reflbo-stats-win/internal/hwinfo/hwinfotest
```

- [ ] **Step 4: Write the sources.** A layout the reader refuses reads as "not running" for the boards: the protocol's four states have no fifth. The page shows the reason (`Detail`). The demo's values depend only on the time, so two demos agree at one moment and the tests can pin them.

`internal/source/source.go`:

```go
// Package source is where readings come from (spec §3, §4): HWiNFO on Windows, the demo anywhere; and what the
// boards get from them: the catalogue of the picked readings and their values (reflbo spec §12a.3).
package source

import (
	"errors"
	"math"
	"time"

	"github.com/Vybo/reflbo-stats-win/internal/config"
	"github.com/Vybo/reflbo-stats-win/internal/hwinfo"
)

// Status is HWiNFO's state as /v1/info and the status messages name it.
type Status string

const (
	OK         Status = "ok"
	NotRunning Status = "not running"
	ShmOff     Status = "shm off"
	Stale      Status = "stale"
)

// Stale after three of HWiNFO's polling periods without a new poll, and never under 5 s: its poll time counts
// whole seconds, and its period is the owner's (HWiNFO's Settings, 2 s by default).
const staleAtLeast = 5 * time.Second

func staleAfter(periodMS uint32) time.Duration {
	return max(staleAtLeast, 3*time.Duration(periodMS)*time.Millisecond)
}

// Item is one reading the source offers, as the settings page lists it.
type Item struct {
	Key   string   `json:"key"`
	Group string   `json:"group"` // HWiNFO's sensor
	Label string   `json:"label"`
	Unit  string   `json:"unit"`
	Type  string   `json:"type"`  // temperature, usage, power …
	Value *float64 `json:"value"` // nil for what isn't a finite number, which JSON can't carry
	Min   *float64 `json:"min"`   // HWiNFO's minimum and maximum since it started
	Max   *float64 `json:"max"`
}

// finite: v, or nil for NaN and the infinities.
func finite(v float64) *float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil
	}
	return &v
}

// Sample is everything the source offers now.
type Sample struct {
	Status Status `json:"status"`
	Detail string `json:"detail,omitempty"` // why, for the page, when HWiNFO can't be read
	Items  []Item `json:"items"`
}

type Source interface {
	Sample() Sample
}

var typeNames = map[hwinfo.ReadingType]string{
	hwinfo.TypeNone: "none", hwinfo.TypeTemp: "temperature", hwinfo.TypeVolt: "voltage", hwinfo.TypeFan: "fan",
	hwinfo.TypeCurrent: "current", hwinfo.TypePower: "power", hwinfo.TypeClock: "clock", hwinfo.TypeUsage: "usage",
	hwinfo.TypeOther: "other",
}

// HWiNFO reads its shared memory each Sample, and calls it stale when HWiNFO's poll time stops moving.
type HWiNFO struct {
	read     func() ([]byte, error)
	running  func() bool
	now      func() time.Time
	lastPoll int64
	moved    time.Time
}

func NewHWiNFO() *HWiNFO {
	return &HWiNFO{read: hwinfo.ReadMapping, running: hwinfo.Running, now: time.Now}
}

func (h *HWiNFO) Sample() Sample {
	b, err := h.read()
	if err != nil {
		switch {
		case errors.Is(err, hwinfo.ErrNoMapping) && h.running():
			return Sample{Status: ShmOff, Items: []Item{}}
		case errors.Is(err, hwinfo.ErrNoMapping), errors.Is(err, hwinfo.ErrDead):
			return Sample{Status: NotRunning, Items: []Item{}}
		default:
			return Sample{Status: NotRunning, Detail: err.Error(), Items: []Item{}}
		}
	}
	snap, err := hwinfo.Parse(b)
	if err != nil {
		if errors.Is(err, hwinfo.ErrDead) {
			return Sample{Status: NotRunning, Items: []Item{}}
		}
		return Sample{Status: NotRunning, Detail: err.Error(), Items: []Item{}}
	}
	now := h.now()
	if snap.PollTime != h.lastPoll || h.moved.IsZero() {
		h.lastPoll, h.moved = snap.PollTime, now
	}
	s := Sample{Status: OK, Items: make([]Item, 0, len(snap.Readings))}
	if now.Sub(h.moved) > staleAfter(snap.PollingPeriodMS) {
		s.Status = Stale
	}
	for _, r := range snap.Readings {
		s.Items = append(s.Items, Item{Key: snap.Key(r), Group: snap.Sensors[r.Sensor].Name, Label: r.Label, Unit: r.Unit,
			Type: typeNames[r.Type], Value: finite(r.Value), Min: finite(r.Min), Max: finite(r.Max)})
	}
	return s
}

// Entry is a picked reading in the catalogue a board gets (reflbo spec §12a.3).
type Entry struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Unit     string   `json:"unit"`
	Decimals int      `json:"decimals"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
}

func Catalog(picked []config.Reading) []Entry {
	out := make([]Entry, len(picked))
	for i, r := range picked {
		out[i] = Entry{ID: r.ID, Label: r.Label, Unit: r.Unit, Decimals: r.Decimals, Min: r.Min, Max: r.Max}
	}
	return out
}

// Values: each picked reading scaled and rounded to its decimals; null when the source hasn't it, or isn't "ok".
func Values(picked []config.Reading, s Sample) map[string]*float64 {
	byKey := make(map[string]*float64, len(s.Items))
	if s.Status == OK {
		for _, it := range s.Items {
			byKey[it.Key] = it.Value
		}
	}
	out := make(map[string]*float64, len(picked))
	for _, r := range picked {
		v := byKey[r.Key]
		if v == nil {
			out[r.ID] = nil
			continue
		}
		p := math.Pow(10, float64(r.Decimals))
		out[r.ID] = finite(math.Round(*v*r.Scale*p) / p) // a scale can overflow what was finite
	}
	return out
}
```


`internal/source/demo.go`:

```go
package source

import (
	"math"
	"time"
)

// Demo is a plausible PC for tests and for a board on this Mac (spec §3): its values depend on the time only,
// so two demos agree at the same moment.
type Demo struct {
	now func() time.Time
}

func NewDemo(now func() time.Time) *Demo {
	if now == nil {
		now = time.Now
	}
	return &Demo{now: now}
}

type demoItem struct {
	key, group, label, unit, typ string
	base, swing, noise, lo, hi   float64
	period                       float64 // seconds
}

var demoItems = []demoItem{
	{"demo:cpu:load", "CPU [#0]: Demo 8-Core", "Total CPU Usage", "%", "usage", 22, 18, 9, 0, 100, 47},
	{"demo:cpu:temp", "CPU [#0]: Demo 8-Core", "CPU (Tctl/Tdie)", "°C", "temperature", 55, 9, 1.5, 20, 100, 61},
	{"demo:cpu:power", "CPU [#0]: Demo 8-Core", "CPU Package Power", "W", "power", 48, 30, 4, 5, 180, 47},
	{"demo:cpu:clock", "CPU [#0]: Demo 8-Core", "Core Clocks (avg)", "MHz", "clock", 4300, 600, 80, 400, 5500, 53},
	{"demo:gpu:load", "GPU [#0]: Demo GPU", "GPU Core Load", "%", "usage", 40, 45, 6, 0, 100, 89},
	{"demo:gpu:temp", "GPU [#0]: Demo GPU", "GPU Temperature", "°C", "temperature", 50, 14, 0.8, 20, 100, 97},
	{"demo:gpu:vram", "GPU [#0]: Demo GPU", "GPU Memory Allocated", "MB", "other", 6300, 1500, 40, 0, 12288, 120},
	{"demo:mem:used", "System: Demo", "Physical Memory Used", "MB", "other", 21800, 2500, 60, 0, 65536, 180},
	{"demo:mem:load", "System: Demo", "Physical Memory Load", "%", "usage", 33, 4, 0.5, 0, 100, 180},
	{"demo:net:down", "Network: Demo Ethernet", "Current DL rate", "KB/s", "other", 9000, 9000, 1500, 0, 120000, 31},
	{"demo:disk:act", "Drive: Demo NVMe", "Total Activity", "%", "usage", 6, 6, 4, 0, 100, 23},
	{"demo:fan:cpu", "Mainboard: Demo", "CPU Fan", "RPM", "fan", 1100, 350, 20, 0, 3000, 61},
}

// noise: a value in [-1, 1] from a time and an index, the same every run.
func noise(sec int64, i int) float64 {
	x := uint64(sec)*0x9e3779b97f4a7c15 ^ uint64(i+1)*0xbf58476d1ce4e5b9
	x ^= x >> 31
	x *= 0x94d049bb133111eb
	x ^= x >> 29
	return float64(x%20001)/10000 - 1
}

func (d *Demo) Sample() Sample {
	t := d.now()
	sec := t.Unix()
	s := Sample{Status: OK, Items: make([]Item, 0, len(demoItems))}
	for i, it := range demoItems {
		phase := float64(t.UnixMilli()%int64(it.period*1000)) / (it.period * 1000) * 2 * math.Pi
		v := it.base + it.swing*math.Sin(phase) + it.noise*noise(sec, i)
		v = math.Max(it.lo, math.Min(it.hi, v))
		lo, hi := it.lo, it.hi
		s.Items = append(s.Items, Item{Key: it.key, Group: it.group, Label: it.label, Unit: it.unit, Type: it.typ,
			Value: &v, Min: &lo, Max: &hi})
	}
	return s
}
```


- [ ] **Step 5: Run them again, and commit.**

Run: `gofmt -l internal; go vet ./... && GOOS=windows go vet ./... && go test ./...`

Expected: every package passes.

```text
ok  	github.com/Vybo/reflbo-stats-win/internal/config	(cached)
ok  	github.com/Vybo/reflbo-stats-win/internal/hwinfo	(cached)
?   	github.com/Vybo/reflbo-stats-win/internal/hwinfo/hwinfotest	[no test files]
ok  	github.com/Vybo/reflbo-stats-win/internal/source	(cached)
```

```bash
git add -A
git commit -m "feat(source): HWiNFO's source and the demo's; the catalogue and the values (spec §3, §4, §5)"
```

### Task 4: The LAN API: `/v1/info` and the stream

**Files:**
- Create: `internal/server/lan.go`, `internal/server/lan_test.go`
- Modify: `go.mod`, `go.sum` (`github.com/coder/websocket`)

**Interfaces:**
- Consumes: `config.Config`, `config.NewToken`, `config.Ungrouped` (Task 1); `source.Source`, `source.Sample`, `source.Status`, `source.Catalog`, `source.Values`, `source.Entry` (Task 3).
- Produces (`internal/server`):
  - `NewHub(cfg config.Config, src source.Source, name, version string) *Hub`;
  - `(*Hub).Run(ctx)`: reads the source at the fastest board's interval, 1 s with none;
  - `(*Hub).Sample() source.Sample`, `(*Hub).Config() config.Config`;
  - `(*Hub).SetConfig(config.Config)`: a new selection reaches every board as a catalogue, a new token closes them all;
  - `(*Hub).Boards() []Board`, with `type Board struct { Addr string; IntervalMS int; Since time.Time }`;
  - `(*Hub).CloseAll()`: Quit, with "going away";
  - `(*Hub).LAN() http.Handler`;
  - the constants `Protocol` (1) and `MaxBoards` (8).

The protocol is reflbo's §12a:
1. The board connects to `/v1/stream` with its token and says `{"type":"hello","protocol":1,"interval_ms":1000}`.
2. The app answers with the catalogue, HWiNFO's status, and then readings at the interval, clamped to 250 ms–10 s.
3. The app pings every 5 s.

What the app closes, and how:
- a board with no hello within 5 s ("no hello");
- another protocol ("protocol N");
- JSON deeper than 4 levels ("too deep");
- a frame over 8 KB (1009);
- every board on a new token ("new token") and on Quit (1001);
- a ninth board gets 503.

- [ ] **Step 1: Write the failing tests.** They run a real server (`httptest`) over a fake source, with `coder/websocket`'s client:
  - the token on both endpoints;
  - hello, catalogue, status and readings at the asked interval, and the clamp;
  - each refusal with its code and reason;
  - a new selection;
  - HWiNFO going stale;
  - a new token;
  - the ninth board;
  - Quit;
  - the source's pace;
  - a scale that overflows: the board gets `null` and stays connected.

```bash
go get github.com/coder/websocket@v1.8.15
```

`internal/server/lan_test.go`:

```go
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
```


- [ ] **Step 2: Run them.**

Run: `go test ./internal/server/`

Expected: the build fails.

```text
FAIL	github.com/Vybo/reflbo-stats-win/internal/server [build failed]
FAIL
# github.com/Vybo/reflbo-stats-win/internal/server [github.com/Vybo/reflbo-stats-win/internal/server.test]
internal/server/lan_test.go:60:10: undefined: Hub
internal/server/lan_test.go:71:9: undefined: NewHub
internal/server/lan_test.go:221:5: undefined: clampInterval
internal/server/lan_test.go:350:18: undefined: MaxBoards
```

- [ ] **Step 3: Write the hub and the handlers.**
  - The hub's lock is never held across I/O.
  - A board's slot is taken under the lock before the upgrade, so boards connecting at once never make nine.
  - Each board has a reader goroutine, which coder/websocket needs for pongs, and a writer that sends only on its ticker or a new hello.
  - A board's close runs once; before the upgrade it only cancels.
  - A write blocked for 5 s ends that board alone.

`internal/server/lan.go`:

```go
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
```


- [ ] **Step 4: Run them again, with the race detector, and commit.**

Run: `gofmt -l internal; go vet ./... && GOOS=windows go vet ./... && go test -race -count=1 ./internal/server/`

Expected: the package passes, race-clean (about 15 s: the tests wait on real intervals).

```text
ok  	github.com/Vybo/reflbo-stats-win/internal/server	Ns
```

`go.mod`:

```diff
--- a/go.mod
+++ b/go.mod
@@ -3,3 +3,5 @@ module github.com/Vybo/reflbo-stats-win
 go 1.26.0
 
 require golang.org/x/sys v0.49.0
+
+require github.com/coder/websocket v1.8.15
```


```bash
go mod tidy
git add -A
git commit -m "feat(server): the LAN API, /v1/info and the stream, with the token and the limits (spec §6; reflbo §12a)"
```

### Task 5: The settings page and its API

**Files:**
- Create: `internal/server/ui.go`, `internal/server/ui_test.go`, `web/embed.go`, `web/index.html`, `web/app.js`, `web/style.css`, `test/web/app.test.mjs`

**Interfaces:**
- Consumes: the hub (Task 4); `config.Save`, `config.Validate`, `config.Grouped`, `config.MakeID`, `config.NewToken`, and the length constants (Task 1); `source.Item` (Task 3).
- Produces:
  - `internal/server`:
    - `type Control interface { Autostart() (bool, error); SetAutostart(bool) error }`;
    - `type UI struct { Hub *Hub; Path string; Control Control; Addresses func() []string; Listen func(port int) error }`;
    - `(*UI).Handler() http.Handler`;
    - `(*UI).SetLAN(port int, err error)`: the LAN listener's state, or why it isn't listening.
  - `web.Files` (`embed.FS`).
  - The page's API:
    - `GET /api/state`: `hwinfo`, `items`, `picked`, `boards`, `connection` {`addresses port listening error ui_port token`}, `app` {`version name autostart config_path`};
    - `PUT /api/readings`;
    - `POST /api/token`;
    - `PUT /api/app` {`autostart`, `port`}.

The guard sits in front of everything. It keeps out what isn't this PC's own browser on this page:
- another Host (DNS rebinding);
- another site's Origin;
- a change sent as anything but JSON, which another site's form can't send without a preflight;
- a body over 64 KB.

It also forbids framing (`X-Frame-Options: DENY`, `frame-ancestors 'none'`), against clickjacking New token.

A newly ticked reading is sent with its key alone. The app fills in what the page leaves out, so these rules live in one place:
- the label and unit from HWiNFO, cut at a character;
- the id from the label;
- a scale left out is 1;
- decimals left out: 0 for %, MHz, MB, RPM, KB/s and MB/s, 3 for V, 1 for the rest;
- 0–100 for a %.

- [ ] **Step 1: Write the API's failing tests.** Against a real server:
  - the state;
  - the static page;
  - a ticked reading's defaults and the decimals and scale it was given;
  - the refusals, with nothing saved;
  - a null list as an empty one;
  - a new token;
  - Start with Windows;
  - a new port opening at once, or reported when it can't be opened;
  - the guard: Host, Origin, Content-Type and framing.

`internal/server/ui_test.go`:

```go
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
```


- [ ] **Step 2: Run them.**

Run: `go test ./internal/server/`

Expected: the build fails.

```text
FAIL	github.com/Vybo/reflbo-stats-win/internal/server [build failed]
FAIL
# github.com/Vybo/reflbo-stats-win/internal/server [github.com/Vybo/reflbo-stats-win/internal/server.test]
internal/server/ui_test.go:25:8: undefined: UI
internal/server/ui_test.go:49:9: undefined: UI
```

- [ ] **Step 3: Write the API.**

`internal/server/ui.go`:

```go
package server

import (
	"encoding/json"
	"errors"
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
	Decimals *int     `json:"decimals"`
	Scale    *float64 `json:"scale"`
}

// withDefaults fills what a newly ticked reading leaves out from what HWiNFO says of it (spec §5).
func withDefaults(in incoming, item *source.Item, taken map[string]bool) config.Reading {
	r := in.Reading
	if in.Decimals != nil {
		r.Decimals = *in.Decimals
	}
	r.Scale = 1 // left out: as HWiNFO has it; a 0 sent is refused by Validate
	if in.Scale != nil {
		r.Scale = *in.Scale
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
```


`web/embed.go`:

```go
// Package web is the settings page (spec §7), embedded in the exe.
package web

import "embed"

//go:embed index.html app.js style.css
var Files embed.FS
```


- [ ] **Step 4: Write the page's failing tests.** They use a DOM just big enough for `app.js`, a fake app, and Node's own runner (no packages):
  - what the page shows;
  - ticking and saving;
  - a refusal keeping the edits;
  - an empty min, a fraction for the scale, and Undo;
  - New token's two clicks;
  - Copy;
  - Start with Windows and the port;
  - the live refresh moving values without touching an edit;
  - HWiNFO's states and reasons;
  - the 32 limit;
  - the search;
  - an edit keeping its field and the same Save button, so the first click saves;
  - the search keeping its box and a collapsed group;
  - a LAN port the app can't open.

`test/web/app.test.mjs`:

```js
// web/app.js against a fake app (spec §7). Node's own test runner, no packages: `node --test test/web/app.test.mjs`.
import assert from 'node:assert/strict';
import fs from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const APP_JS = fs.readFileSync(fileURLToPath(new URL('../../web/app.js', import.meta.url)), 'utf8');

/* ---- a DOM just big enough for app.js ---- */

class FakeElement {
  constructor(tag) {
    Object.assign(this, { tag, children: [], attrs: {}, listeners: {}, className: '', textContent: '', value: '',
                          hidden: false, disabled: false, checked: false, open: false, parent: null });
  }
  append(...kids) {
    for (const k of kids) {
      if (k instanceof FakeElement) k.parent = this;
      this.children.push(k);
    }
  }
  replaceChildren(...kids) { this.children = []; this.append(...kids); }
  replaceWith(el) {
    const p = this.parent;
    if (!p) return;
    p.children[p.children.indexOf(this)] = el;
    el.parent = p;
  }
  setAttribute(k, v) { this.attrs[k] = v; }
  getAttribute(k) { return this.attrs[k]; }
  addEventListener(type, fn) { (this.listeners[type] ||= []).push(fn); }
  async fire(type, target = this) {
    await Promise.all((this.listeners[type] || []).map((fn) => fn({ target, preventDefault() {} })));
    await settle();
  }
  click() { return this.fire('click'); }
}

function below(el) {
  return [el, ...el.children.filter((k) => k instanceof FakeElement).flatMap(below)];
}

function text(el) {
  return typeof el === 'string' ? el : el.textContent + el.children.map(text).join('');
}

const settle = () => new Promise((r) => setTimeout(r, 0));

/* ---- the app ---- */

function baseState() {
  return {
    hwinfo: { status: 'ok', detail: '' },
    items: [
      { key: 'k:cpu', group: 'CPU [#0]: Demo', label: 'Total CPU Usage', unit: '%', type: 'usage', value: 12.4 },
      { key: 'k:temp', group: 'CPU [#0]: Demo', label: 'CPU (Tctl/Tdie)', unit: '°C', type: 'temperature', value: 54.26 },
      { key: 'k:ram', group: 'System: Demo', label: 'Physical Memory Used', unit: 'MB', type: 'other', value: 21811 },
    ],
    picked: [
      { key: 'k:cpu', id: 'cpu_load', label: 'CPU load', unit: '%', scale: 1, decimals: 0, min: 0, max: 100 },
      { key: 'k:ram', id: 'ram_used', label: 'RAM', unit: 'GB', scale: 1 / 1024, decimals: 1 },
    ],
    boards: [],
    connection: { addresses: ['192.168.1.20'], port: 47123, listening: 47123, ui_port: 47124, token: 'ABCDE-FGHIJ-KLMNO-PQRST-UVWXY-Z' },
    app: { version: '1.0.0', name: 'DESKTOP-TEST', autostart: false, config_path: 'C:\\Users\\x\\AppData\\Roaming\\reflbo-stats\\config.json' },
  };
}

function reply(status, body) {
  return { status, ok: status >= 200 && status < 300, json: async () => body };
}

async function open(state = baseState(), handlers = {}) {
  const els = { app: new FakeElement('main'), pill: new FakeElement('span') };
  const calls = [];
  let tick = null;
  const clipboard = [];
  const sandbox = {
    document: { createElement: (tag) => new FakeElement(tag), getElementById: (id) => els[id] },
    fetch: async (path, opts = {}) => {
      const method = opts.method || 'GET';
      const body = opts.body === undefined ? undefined : JSON.parse(opts.body);
      calls.push({ method, path, body, type: (opts.headers || {})['Content-Type'] });
      const handler = handlers[`${method} ${path}`];
      if (handler) return handler(body, state);
      if (method === 'GET' && path === '/api/state') return reply(200, state);
      return reply(404, { error: 'no such API' });
    },
    setInterval: (fn) => { tick = fn; },
    navigator: { clipboard: { writeText: async (t) => { clipboard.push(t); } } },
    console, JSON, Math, Number, String, Map, Promise, Date, Object, Error, RegExp,
  };
  vm.createContext(sandbox);
  vm.runInContext(APP_JS, sandbox);
  await settle();
  await settle();
  return { root: els.app, pill: els.pill, calls, clipboard, tick: async () => { await tick(); await settle(); },
           setState: (s) => { state = s; } };
}

const all = (root, tag) => below(root).filter((e) => e.tag === tag);
const button = (root, name) => {
  const b = all(root, 'button').find((e) => text(e) === name);
  assert.ok(b, `no button "${name}"`);
  return b;
};
const checkbox = (root, label) => {
  const l = all(root, 'label').find((e) => text(e).includes(label));
  assert.ok(l, `no reading "${label}"`);
  return l.children.find((k) => k.tag === 'input');
};

test('the page shows HWiNFO, the picked readings as the boards get them, and the sensors', async () => {
  const p = await open();
  const page = text(p.root);
  assert.match(page, /HWiNFO is running, and its readings come in\./);
  assert.equal(p.pill.textContent, 'HWiNFO ok');
  assert.match(page, /Readings the boards get \(2 of 32\)/);
  assert.match(page, /12 %/); // cpu_load at 0 decimals
  assert.match(page, /21\.3 GB/); // 21811 MB × 1/1024 at 1 decimal
  assert.match(page, /CPU \[#0\]: Demo \(2\)/);
  assert.match(page, /System: Demo \(1\)/);
  assert.equal(checkbox(p.root, 'Total CPU Usage').checked, true);
  assert.equal(checkbox(p.root, 'CPU (Tctl/Tdie)').checked, false);
  const scale = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'scale' && e.value === '1/1024');
  assert.ok(scale, 'the RAM scale shows as 1/1024');
  assert.equal(button(p.root, 'Save').disabled, true);
});

test('ticking a reading and saving sends it for the app to fill in', async () => {
  const saved = [];
  const p = await open(baseState(), {
    'PUT /api/readings': (body, state) => {
      saved.push(body);
      const next = { ...state, picked: [...body.slice(0, 2), { key: 'k:temp', id: 'cpu_tctl_tdie', label: 'CPU (Tctl/Tdie)', unit: '°C', scale: 1, decimals: 1 }] };
      return reply(200, next);
    },
  });
  const box = checkbox(p.root, 'CPU (Tctl/Tdie)');
  box.checked = true;
  await box.fire('change');
  assert.match(text(p.root), /\(on save\)/);
  assert.equal(button(p.root, 'Save').disabled, false);
  await button(p.root, 'Save').click();
  assert.deepEqual(saved[0][2], { key: 'k:temp' });
  assert.equal(p.calls.at(-1).type, 'application/json');
  assert.match(text(p.root), /Saved\. The boards have the new list\./);
  assert.match(text(p.root), /Readings the boards get \(3 of 32\)/);
  assert.equal(button(p.root, 'Save').disabled, true);
});

test('a refusal shows the app\'s reason and keeps the edits', async () => {
  const p = await open(baseState(), {
    'PUT /api/readings': () => reply(400, { error: 'id "Bad": 1-15 characters of a-z, 0-9 and _' }),
  });
  const id = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'id' && e.value === 'cpu_load');
  id.value = 'Bad';
  await id.fire('input');
  await button(p.root, 'Save').click();
  assert.match(text(p.root), /id "Bad": 1-15 characters of a-z, 0-9 and _/);
  assert.equal(button(p.root, 'Save').disabled, false);
  const still = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'id' && e.value === 'Bad');
  assert.ok(still, 'the edit stays');
});

test('an empty min goes as none, a fraction as its value, and Undo brings the saved list back', async () => {
  const sent = [];
  const p = await open(baseState(), { 'PUT /api/readings': (body, state) => { sent.push(body); return reply(200, state); } });
  const min = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'min' && e.value === '0');
  min.value = '';
  await min.fire('input');
  const scale = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'scale' && e.value === '1');
  scale.value = '1/1000';
  await scale.fire('input');
  await button(p.root, 'Save').click();
  assert.equal('min' in sent[0][0], false);
  assert.equal(sent[0][0].scale, 0.001);
  const label = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'label' && e.value === 'CPU load');
  label.value = 'Changed';
  await label.fire('input');
  await button(p.root, 'Undo changes').click();
  assert.ok(all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'label' && e.value === 'CPU load'));
});

test('a new token needs a second click, and Cancel keeps the old one', async () => {
  let posts = 0;
  const p = await open(baseState(), {
    'POST /api/token': (body, state) => { posts++; return reply(200, { ...state, connection: { ...state.connection, token: 'ZZZZZ-ZZZZZ-ZZZZZ-ZZZZZ-ZZZZZ-Z' } }); },
  });
  await button(p.root, 'New token').click();
  assert.equal(posts, 0);
  await button(p.root, 'Cancel').click();
  assert.match(text(p.root), /New token/);
  await button(p.root, 'New token').click();
  await button(p.root, 'Confirm: every board needs the new token').click();
  assert.equal(posts, 1);
  assert.match(text(p.root), /ZZZZZ-ZZZZZ-ZZZZZ-ZZZZZ-ZZZZZ-Z/);
});

test('Copy puts the token on the clipboard', async () => {
  const p = await open();
  await button(p.root, 'Copy').click();
  assert.deepEqual(p.clipboard, ['ABCDE-FGHIJ-KLMNO-PQRST-UVWXY-Z']);
  assert.match(text(p.root), /Copied/);
});

test('Start with Windows and the port go to the app', async () => {
  const puts = [];
  const p = await open(baseState(), {
    'PUT /api/app': (body, state) => {
      puts.push(body);
      const next = clone(state);
      if (body.autostart !== undefined) next.app.autostart = body.autostart;
      if (body.port !== undefined) next.connection.port = body.port;
      return reply(200, next);
    },
  });
  const auto = checkbox(p.root, 'Start with Windows');
  auto.checked = true;
  await auto.fire('change');
  assert.deepEqual(puts[0], { autostart: true });
  const port = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'port');
  port.value = '48000';
  await button(p.root, 'Apply').click();
  assert.deepEqual(puts[1], { port: 48000 });
  assert.doesNotMatch(text(p.root), /after a restart/);
});

function clone(x) { return JSON.parse(JSON.stringify(x)); }

test('the live refresh moves the values and keeps an edit in progress', async () => {
  const state = baseState();
  const p = await open(state);
  const label = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'label' && e.value === 'CPU load');
  label.value = 'Mine';
  await label.fire('input');
  const next = clone(state);
  next.items[0].value = 87.6;
  next.boards = [{ addr: '192.168.1.66:51000', interval_ms: 1000, since: '2026-10-19T12:00:00Z' }];
  p.setState(next);
  await p.tick();
  assert.match(text(p.root), /88 %/);
  assert.match(text(p.root), /87\.6 %/);
  assert.match(text(p.root), /192\.168\.1\.66:51000/);
  assert.ok(all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'label' && e.value === 'Mine'), 'the edit stays');
  assert.equal(button(p.root, 'Save').disabled, false);
});

test('HWiNFO\'s other states say what to do', async () => {
  for (const [status, detail, words] of [['not running', '', /Start it \(Sensors-only is enough\)/],
    ['not running', 'HWiNFO\'s shared memory version 9 is unknown to this app', /version 9 is unknown to this app/],
    ['shm off', '', /Shared Memory Support is off/], ['stale', '', /stopped updating/]]) {
    const s = baseState();
    s.hwinfo = { status, detail };
    const p = await open(s);
    assert.match(text(p.root), words);
    assert.equal(p.pill.textContent, `HWiNFO ${status}`);
    assert.match(text(p.root), /—/); // the boards get no value
    if (detail) assert.doesNotMatch(text(p.root), /Start it/); // the reason says more than the advice
  }
});

test('32 picked readings leave the other boxes off', async () => {
  const s = baseState();
  s.picked = Array.from({ length: 32 }, (_, i) => ({ key: `x:${i}`, id: `r${i}`, label: `R${i}`, unit: '', scale: 1, decimals: 0 }));
  s.picked[0].key = 'k:cpu';
  const p = await open(s);
  assert.equal(checkbox(p.root, 'CPU (Tctl/Tdie)').disabled, true);
  assert.equal(checkbox(p.root, 'Total CPU Usage').disabled, false);
  assert.match(text(p.root), /32 readings at most\./);
});

test('the search narrows the sensors', async () => {
  const p = await open();
  const search = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'search');
  search.value = 'memory';
  await search.fire('input');
  assert.match(text(p.root), /Physical Memory Used/);
  assert.doesNotMatch(text(p.root), /Total CPU Usage/);
});

test('an edit keeps the field it is typed in and enables the same Save button', async () => {
  const sent = [];
  const p = await open(baseState(), { 'PUT /api/readings': (body, state) => { sent.push(body); return reply(200, state); } });
  const label = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'label' && e.value === 'CPU load');
  const save = button(p.root, 'Save');
  label.value = 'CPU busy';
  await label.fire('input');
  assert.ok(below(p.root).includes(label), 'the field being typed in is still the same one');
  assert.ok(below(p.root).includes(save), 'the Save button is still the same one');
  assert.equal(save.disabled, false);
  await save.click(); // the first click after the edit saves
  assert.equal(sent.length, 1);
  assert.equal(sent[0][0].label, 'CPU busy');
});

test('the search keeps its box, and a collapsed group stays collapsed', async () => {
  const p = await open();
  const system = all(p.root, 'details').find((d) => text(d).startsWith('System: Demo'));
  system.open = false;
  await system.fire('toggle');
  const search = all(p.root, 'input').find((e) => e.attrs['aria-label'] === 'search');
  search.value = 'cpu';
  await search.fire('input');
  assert.ok(below(p.root).includes(search), 'the search box is still the same one');
  search.value = '';
  await search.fire('input');
  const again = all(p.root, 'details').find((d) => text(d).startsWith('System: Demo'));
  assert.equal(again.open, false);
  assert.equal(all(p.root, 'details').find((d) => text(d).startsWith('CPU [#0]')).open, true);
});

test('a LAN port the app can\'t open says so', async () => {
  const s = baseState();
  s.connection = { ...s.connection, listening: 0, error: 'listen tcp :47123: bind: address already in use' };
  const p = await open(s);
  assert.match(text(p.root), /The boards can't reach this PC: listen tcp :47123: bind: address already in use\. Choose another LAN port under App, and Apply\./);
  assert.match(text(p.root), /port —/);
});
```


- [ ] **Step 5: Run them.**

Run: `node --test test/web/app.test.mjs 2>&1 | grep -E '^# (tests|pass|fail)'`

Expected: the file fails as a whole, as there is no `web/app.js` to load yet.

```text
# tests 1
# pass 0
# fail 1
```

- [ ] **Step 6: Write the page.**
  - It's plain JS with no build step.
  - An edit updates Save, Undo and the message in place. Only adding, moving or removing a reading redraws the table: a redraw would take the focus from the field being typed in, and the Save button from under the click that ends the edit.
  - The search redraws the list under its box.
  - `fill()` and `h()` flatten their children fully and drop `null`. A browser would print a `null` passed to `replaceChildren` as text.
  - New token takes a second click rather than `confirm()`, which would block a page driven over CDP.

`web/index.html`:

```html
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>reflbo-stats</title>
<link rel="stylesheet" href="style.css">
</head>
<body>
<header><h1>reflbo-stats</h1><span id="pill" class="pill">…</span></header>
<main id="app"><p class="muted">Loading…</p></main>
<script src="app.js"></script>
</body>
</html>
```


`web/style.css`:

```css
:root {
  --bg: #f4f5f2; --card: #ffffff; --fg: #1d201c; --muted: #61665e; --line: #d6d9d2; --accent: #2f5d50;
  --ok: #2c7a4b; --warn: #a06000; --bad: #b3261e;
  font: 15px/1.45 system-ui, -apple-system, "Segoe UI", sans-serif;
  color-scheme: light dark;
}
@media (prefers-color-scheme: dark) {
  :root { --bg: #151714; --card: #1e211d; --fg: #e5e8e2; --muted: #a0a69b; --line: #363a34; --accent: #8cc4b2;
          --ok: #6fcf97; --warn: #f2c94c; --bad: #ff8a80; }
}
body { margin: 0; background: var(--bg); color: var(--fg); }
header { display: flex; align-items: center; gap: 12px; padding: 14px 20px; border-bottom: 1px solid var(--line); }
header h1 { font-size: 18px; margin: 0; }
main { max-width: 920px; margin: 0 auto; padding: 16px 20px 48px; display: grid; gap: 16px; }
section { background: var(--card); border: 1px solid var(--line); border-radius: 8px; padding: 14px 16px; }
h2 { font-size: 15px; margin: 0 0 10px; }
.muted { color: var(--muted); }
.pill { font-size: 12px; padding: 2px 8px; border-radius: 10px; border: 1px solid currentColor; }
.pill.ok { color: var(--ok); } .pill.warn { color: var(--warn); } .pill.bad { color: var(--bad); }
.error { color: var(--bad); }
input, select, button { font: inherit; color: inherit; }
input[type=text], input[type=number], select { background: transparent; border: 1px solid var(--line); border-radius: 4px; padding: 3px 6px; }
button { background: var(--card); border: 1px solid var(--line); border-radius: 4px; padding: 4px 10px; cursor: pointer; }
button.primary { background: var(--accent); color: var(--bg); border-color: var(--accent); }
button:disabled { opacity: .5; cursor: default; }
:focus-visible { outline: 2px solid var(--accent); outline-offset: 1px; }
details { border-top: 1px solid var(--line); padding: 4px 0; }
summary { cursor: pointer; padding: 4px 0; }
.item { display: grid; grid-template-columns: 24px 1fr auto; gap: 8px; align-items: center; padding: 2px 0; }
.value { font-variant-numeric: tabular-nums; color: var(--muted); }
table { width: 100%; border-collapse: collapse; }
th, td { text-align: left; padding: 4px 6px; border-bottom: 1px solid var(--line); vertical-align: middle; }
td input[type=text] { width: 9em; } td input[type=number] { width: 5.5em; }
.row { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
code { font: 13px ui-monospace, Menlo, Consolas, monospace; }
.scroll { overflow-x: auto; }
```


`web/app.js`:

```js
'use strict';
/* The settings page (spec §7): plain JS, no build step. Its API, on 127.0.0.1 only: GET /api/state,
 * PUT /api/readings, POST /api/token, PUT /api/app. Live values refresh every second without touching the
 * fields being edited. */

const MAX_READINGS = 32;

const HWINFO = {
  ok: ['ok', 'HWiNFO is running, and its readings come in.'],
  'not running': ['bad', 'HWiNFO isn\'t running. Start it (Sensors-only is enough), with Settings ▸ Shared Memory Support on.'],
  'shm off': ['bad', 'HWiNFO runs, but its Shared Memory Support is off: turn it on in HWiNFO\'s Settings.'],
  stale: ['warn', 'HWiNFO has stopped updating its readings.'],
};

let state = null;      // the last /api/state
let picked = [];       // the editor's copy of the picked readings
let dirty = false;     // picked differs from the saved list
let filter = '';
let confirming = false; // New token waits for its second click
let message = null;    // { text, error } after a save
let appMessage = null; // the same for the App section
const closedGroups = new Set(); // the sensor groups the owner collapsed
const shown = new Map(); // a reading's key -> the elements showing its live value
const parts = {};      // the sections and controls updated in place

function h(tag, attrs = {}, ...kids) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v === null || v === undefined || v === false) continue;
    if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
    else if (k === 'text') el.textContent = v;
    else if (k === 'class') el.className = v;
    else if (k === 'value' || k === 'checked' || k === 'disabled' || k === 'open') el[k] = v;
    else el.setAttribute(k, v === true ? '' : String(v));
  }
  el.append(...kids.flat(Infinity).filter((k) => k !== null && k !== undefined && k !== false));
  return el;
}

/* replaceChildren without the null and false a condition leaves (a browser would print them). */
function fill(el, ...kids) {
  el.replaceChildren(...kids.flat(Infinity).filter((k) => k !== null && k !== undefined && k !== false));
}

async function api(method, path, body) {
  const opts = { method, headers: {} };
  if (body !== undefined) {
    opts.headers['Content-Type'] = 'application/json';
    opts.body = JSON.stringify(body);
  }
  const res = await fetch(path, opts);
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new Error(data.error || `The app answered ${res.status}.`);
  return data;
}

function clone(x) { return JSON.parse(JSON.stringify(x)); }

function number(v, decimals) {
  if (v === null || v === undefined || !Number.isFinite(v)) return '—';
  return decimals === undefined ? String(Math.round(v * 100) / 100) : v.toFixed(decimals);
}

/* A scale as typed, a number or a fraction ("1/1024"); NaN for anything else. */
function parseScale(text) {
  const m = /^\s*([0-9]*\.?[0-9]+)\s*(?:\/\s*([0-9]*\.?[0-9]+))?\s*$/.exec(text);
  if (!m) return NaN;
  return m[2] === undefined ? Number(m[1]) : Number(m[1]) / Number(m[2]);
}

/* And shown back: a whole fraction as one ("1/1024"), else the number. */
function showScale(scale) {
  if (scale === undefined || scale === null) return '';
  const inverse = 1 / scale;
  return scale < 1 && Math.abs(inverse - Math.round(inverse)) < 1e-9 ? `1/${Math.round(inverse)}` : String(scale);
}

function itemByKey(key) { return state.items.find((it) => it.key === key); }

/* What a board gets for a picked reading: HWiNFO's value scaled, at its decimals. */
function boardValue(r) {
  const it = itemByKey(r.key);
  if (!it || state.hwinfo.status !== 'ok') return '—';
  return `${number(it.value * (r.scale || 1), r.decimals)} ${r.unit || it.unit}`.trim();
}

/* An edit only updates Save, Undo and the message in place: redrawing the table would take the focus from the
 * field being typed in, and the Save button from under the click that ends the edit. */
function setDirty(on) {
  dirty = on;
  message = null;
  if (parts.save) {
    parts.save.disabled = !dirty;
    parts.undo.disabled = !dirty;
    parts.msg.className = 'muted';
    parts.msg.textContent = '';
  }
}

/* A change to the list itself (a reading added, moved or removed) redraws it. */
function restructure() {
  setDirty(true);
  renderPicked();
}

/* ---- sections ---- */

function hwinfoSection() {
  const [, text] = HWINFO[state.hwinfo.status] || ['bad', state.hwinfo.status];
  return h('section', {}, h('h2', { text: 'HWiNFO' }), // the reason first when there is one: it says more
    state.hwinfo.detail ? h('p', { class: 'error', text: state.hwinfo.detail }) : h('p', { text }));
}

function field(r, name, kind, attrs = {}) {
  const value = r[name] === undefined || r[name] === null ? '' : name === 'scale' ? showScale(r[name]) : r[name];
  return h('input', { type: kind, value: String(value), 'aria-label': name, ...attrs, oninput: (ev) => {
    const raw = ev.target.value.trim();
    if (name === 'scale') {
      r.scale = raw === '' ? 1 : parseScale(raw);
    } else if (kind === 'number') {
      if (raw === '') delete r[name];
      else r[name] = Number(raw);
    } else {
      r[name] = raw;
    }
    setDirty(true);
  } });
}

function move(i, by) {
  const j = i + by;
  if (j < 0 || j >= picked.length) return;
  [picked[i], picked[j]] = [picked[j], picked[i]];
  restructure();
}

function pickedRows() {
  shown.forEach((list, key) => shown.set(key, list.filter((el) => !el.picked)));
  return picked.map((r, i) => {
    const it = itemByKey(r.key);
    const live = h('td', { class: 'value', text: boardValue(r) });
    live.picked = true;
    shown.set(r.key, [...(shown.get(r.key) || []), live]);
    return h('tr', {},
      h('td', {}, r.id ? field(r, 'id', 'text', { maxlength: 15 }) : h('span', { class: 'muted', text: '(on save)' })),
      h('td', {}, field(r, 'label', 'text', { maxlength: 23, placeholder: it ? it.label : '' }),
        it ? null : h('div', { class: 'error', text: 'Not in HWiNFO now' })),
      h('td', {}, field(r, 'unit', 'text', { maxlength: 7, placeholder: it ? it.unit : '' })),
      h('td', {}, field(r, 'scale', 'text', { placeholder: '1' })),
      h('td', {}, h('select', { 'aria-label': 'decimals', onchange: (ev) => { r.decimals = Number(ev.target.value); setDirty(true); } },
        [0, 1, 2, 3].map((d) => h('option', { value: String(d), selected: (r.decimals || 0) === d ? true : null, text: String(d) })))),
      h('td', {}, field(r, 'min', 'number', { step: 'any' })),
      h('td', {}, field(r, 'max', 'number', { step: 'any' })),
      live,
      h('td', {}, h('button', { text: '↑', 'aria-label': 'up', disabled: i === 0, onclick: () => move(i, -1) }),
        h('button', { text: '↓', 'aria-label': 'down', disabled: i === picked.length - 1, onclick: () => move(i, 1) }),
        h('button', { text: '✕', 'aria-label': 'remove', onclick: () => { picked.splice(i, 1); restructure(); renderSensorList(); } })));
  });
}

async function save() {
  try {
    state = await api('PUT', '/api/readings', picked);
    picked = clone(state.picked);
    dirty = false;
    message = { text: 'Saved. The boards have the new list.' };
  } catch (e) {
    message = { text: e.message, error: true };
  }
  render();
}

function undo() {
  picked = clone(state.picked);
  dirty = false;
  message = null;
  render();
}

function renderPicked() {
  const body = picked.length
    ? h('div', { class: 'scroll' }, h('table', {},
      h('tr', {}, ['Id', 'Label', 'Unit', 'Scale', 'Decimals', 'Min', 'Max', 'Value', ''].map((t) => h('th', { text: t }))),
      pickedRows()))
    : h('p', { class: 'muted', text: 'Tick readings below to send them to the boards.' });
  parts.save = h('button', { class: 'primary', text: 'Save', disabled: !dirty, onclick: save });
  parts.undo = h('button', { text: 'Undo changes', disabled: !dirty, onclick: undo });
  parts.msg = h('span', { class: message && message.error ? 'error' : 'muted', text: message ? message.text : '' });
  fill(parts.picked,
    h('h2', { text: `Readings the boards get (${picked.length} of ${MAX_READINGS})` }),
    h('p', { class: 'muted', text: 'The boards name each one pc.<id>. Scale multiplies HWiNFO\'s value: 1/1024 turns MB into GB.' }),
    body,
    h('div', { class: 'row' }, parts.save, parts.undo, parts.msg));
}

function toggle(item, on) {
  if (on) picked.push({ key: item.key });
  else picked = picked.filter((r) => r.key !== item.key);
  restructure();
  renderSensorList();
}

/* The sensors' list redraws as the search narrows it; the search box itself stays, with its focus. */
function renderSensorList() {
  const groups = new Map();
  const f = filter.toLowerCase();
  for (const it of state.items) {
    if (f && !`${it.group} ${it.label}`.toLowerCase().includes(f)) continue;
    if (!groups.has(it.group)) groups.set(it.group, []);
    groups.get(it.group).push(it);
  }
  shown.forEach((list, key) => shown.set(key, list.filter((el) => el.picked)));
  const full = picked.length >= MAX_READINGS;
  const list = [...groups].map(([group, its]) => h('details', { open: !closedGroups.has(group), ontoggle: (ev) => {
    if (ev.target.open) closedGroups.delete(group);
    else closedGroups.add(group);
  } },
    h('summary', { text: `${group} (${its.length})` }),
    its.map((it) => {
      const on = picked.some((r) => r.key === it.key);
      const value = h('span', { class: 'value', text: `${number(it.value)} ${it.unit}`.trim() });
      shown.set(it.key, [...(shown.get(it.key) || []), value]);
      return h('label', { class: 'item' },
        h('input', { type: 'checkbox', checked: on, disabled: full && !on, onchange: (ev) => toggle(it, ev.target.checked) }),
        h('span', { text: it.label }), value);
    })));
  fill(parts.sensorList,
    full ? h('p', { class: 'muted', text: `${MAX_READINGS} readings at most.` }) : null,
    list.length ? list : h('p', { class: 'muted', text: state.items.length ? 'Nothing matches.' : 'No readings yet.' }));
}

function renderSensors() {
  parts.sensorList = h('div', {});
  fill(parts.sensors,
    h('h2', { text: 'HWiNFO\'s readings' }),
    h('input', { type: 'text', placeholder: 'Search', value: filter, 'aria-label': 'search', oninput: (ev) => {
      filter = ev.target.value;
      renderSensorList();
    } }),
    parts.sensorList);
  renderSensorList();
}

function boardsSection() {
  const boards = state.boards || [];
  return h('section', {}, h('h2', { text: 'Boards' }),
    boards.length
      ? h('table', {}, h('tr', {}, ['Address', 'Interval', 'Since'].map((t) => h('th', { text: t }))),
        boards.map((b) => h('tr', {}, h('td', { text: b.addr }), h('td', { text: `${b.interval_ms} ms` }),
          h('td', { text: new Date(b.since).toLocaleTimeString() }))))
      : h('p', { class: 'muted', text: 'No board is connected. If one should be, allow reflbo-stats through Windows\' firewall for private networks.' }));
}

async function newToken() {
  if (!confirming) {
    confirming = true;
    render();
    return;
  }
  confirming = false;
  try {
    state = await api('POST', '/api/token', {});
    message = null;
  } catch (e) {
    message = { text: e.message, error: true };
  }
  render();
}

async function copyToken() {
  try {
    await navigator.clipboard.writeText(state.connection.token);
    parts.copied.textContent = 'Copied';
  } catch (e) {
    parts.copied.textContent = 'Select the token and copy it';
  }
}

function connectionSection() {
  const c = state.connection;
  parts.copied = h('span', { class: 'muted' });
  return h('section', {}, h('h2', { text: 'Connection' }),
    c.error ? h('p', { class: 'error', text: `The boards can't reach this PC: ${c.error}. Choose another LAN port under App, and Apply.` }) : null,
    h('p', { text: 'Enter these on reflbo\'s PC page:' }),
    h('p', {}, 'Address: ', (c.addresses || []).length ? c.addresses.map((a, i) => [i ? ' or ' : '', h('code', { text: a })]) : h('span', { class: 'muted', text: 'no network' }),
      ', port ', h('code', { text: c.listening ? String(c.listening) : '—' })),
    h('p', { class: 'row' }, 'Token: ', h('code', { text: c.token }),
      h('button', { text: 'Copy', onclick: copyToken }), parts.copied,
      h('button', { text: confirming ? 'Confirm: every board needs the new token' : 'New token', onclick: newToken }),
      confirming ? h('button', { text: 'Cancel', onclick: () => { confirming = false; render(); } }) : null));
}

async function putApp(body) {
  try {
    state = await api('PUT', '/api/app', body);
    appMessage = null;
  } catch (e) {
    appMessage = { text: e.message, error: true };
  }
  render();
}

function appSection() {
  const a = state.app;
  const c = state.connection;
  const port = h('input', { type: 'number', value: String(c.port), min: 1024, max: 65535, 'aria-label': 'port' });
  return h('section', {}, h('h2', { text: 'App' }),
    h('label', { class: 'row' }, h('input', { type: 'checkbox', checked: a.autostart, onchange: (ev) => putApp({ autostart: ev.target.checked }) }),
      'Start with Windows'),
    h('p', { class: 'row' }, 'LAN port ', port, h('button', { text: 'Apply', onclick: () => putApp({ port: Number(port.value) }) }),
      appMessage ? h('span', { class: 'error', text: appMessage.text }) : null),
    h('p', { class: 'muted', text: `Version ${a.version} on ${a.name}. Settings in ${a.config_path}.` }));
}

function renderPill() {
  const pill = document.getElementById('pill');
  const [cls] = HWINFO[state.hwinfo.status] || ['bad'];
  pill.className = `pill ${cls}`;
  pill.textContent = `HWiNFO ${state.hwinfo.status}`;
}

function render() {
  renderPill();
  shown.clear();
  parts.hwinfo = hwinfoSection();
  parts.picked = h('section', {});
  parts.sensors = h('section', {});
  parts.boards = boardsSection();
  renderPicked();
  renderSensors();
  fill(document.getElementById('app'), parts.hwinfo, parts.picked, parts.sensors, parts.boards,
    connectionSection(), appSection());
}

/* The live refresh: the values, HWiNFO's state and the boards; the editor and the search stay as they are. */
async function refresh() {
  let next;
  try {
    next = await api('GET', '/api/state');
  } catch (e) {
    document.getElementById('pill').textContent = 'The app isn\'t answering';
    return;
  }
  const savedChanged = JSON.stringify(next.picked) !== JSON.stringify(state.picked);
  state = next;
  if (savedChanged && !dirty) {
    picked = clone(state.picked);
    renderPicked();
  }
  renderPill();
  const hw = hwinfoSection();
  parts.hwinfo.replaceWith(hw);
  parts.hwinfo = hw;
  const boards = boardsSection();
  parts.boards.replaceWith(boards);
  parts.boards = boards;
  for (const it of state.items) {
    for (const el of shown.get(it.key) || []) {
      el.textContent = el.picked ? boardValue(picked.find((r) => r.key === it.key) || { key: it.key })
        : `${number(it.value)} ${it.unit}`.trim();
    }
  }
}

async function start() {
  try {
    state = await api('GET', '/api/state');
  } catch (e) {
    document.getElementById('app').replaceChildren(h('p', { class: 'error', text: `The app isn't answering: ${e.message}` }));
    return;
  }
  picked = clone(state.picked);
  render();
  setInterval(refresh, 1000);
}

start();
```


- [ ] **Step 7: Run everything, and commit.**

Run: `gofmt -l . ; go vet ./... && go test -race -count=1 ./internal/server/ && node --test test/web/app.test.mjs 2>&1 | grep -E '^# (tests|pass|fail)'`

Expected: both pass.

```text
ok  	github.com/Vybo/reflbo-stats-win/internal/server	Ns
# tests 14
# pass 14
# fail 0
```

```bash
git add -A
git commit -m "feat(server): the settings page and its API on the loopback interface (spec §7)"
```

### Task 6: The app as a whole

**Files:**
- Create:
  - `internal/app/app.go`, `internal/app/app_test.go`, `internal/app/errno_windows.go`, `internal/app/errno_other.go`
  - `internal/autostart/autostart.go`, `internal/autostart/autostart_test.go`, `internal/autostart/run_windows.go`, `internal/autostart/run_other.go`
  - `internal/icon/icon.go`, `internal/icon/icon_test.go`
  - `cmd/reflbo-stats/main.go`, `cmd/reflbo-stats/tray.go`, `cmd/reflbo-stats/browser.go`
- Modify: `web/style.css` (the page at a desktop's width), `go.mod`, `go.sum` (`fyne.io/systray`)

**Interfaces:**
- Consumes: everything before: `config.LoadOrBak`, `config.Save`, `config.Default`, `config.Path`; `source.NewHWiNFO`, `source.NewDemo`; `server.NewHub`, `(*Hub).Run`, `(*Hub).LAN`, `(*Hub).CloseAll`, `server.UI`, `(*UI).SetLAN`; `hwinfo.ReadMapping`.
- Produces:
  - `internal/app`:
    - `Start(ctx, Options) (*App, error)`, with `type Options struct { ConfigPath string; Demo bool; Version, Name string; Control server.Control; Log *log.Logger }`;
    - `ErrRunning`, for a second copy;
    - `(*App).Quit()`, `(*App).Done()`, `(*App).Listening() bool`;
    - `App.Hub`, `App.UIURL`;
    - `Addresses() []string`.
  - `internal/autostart`: `New(exe string) *Control`, which implements `server.Control` through the per-user Run key.
  - `internal/icon`: `PNG(State) []byte`, `ICO(State) []byte`, with the states `Normal`, `Warning`, `Connected`.
  - The command: `reflbo-stats [-demo] [-no-tray] [-config path] [-dump file] [-version]`.

How the app starts, and stops:
- **The settings page's port** comes first. A second copy can't listen on it (`ErrRunning`), so it opens the running one's page and exits. That's the one-instance rule.
- **The LAN port:** one that another program holds doesn't stop the app. The page says why under Connection, opens by itself at a start, and Apply on a new port opens it at once.
- **Quit** closes the boards' connections cleanly first, so they end their sessions at once.
- **The tray** is a thin layer, not tested here: left-click opens the page; right-click has Open settings, Start with Windows and Quit. Its icon shows HWiNFO's trouble or a connected board, and its tick follows the page's checkbox.
- **The log:** the exe has no console under `-H windowsgui`, so it logs to `log.txt` beside the settings, started afresh past 1 MB; `-dump` reports there too.

- [ ] **Step 1: Start with Windows, test first.** The Run key holds the exe's path quoted. An entry naming another copy counts as off, so turning it on points it here.

`internal/autostart/autostart_test.go`:

```go
package autostart

import "testing"

type fakeRun map[string]string

func (f fakeRun) get(name string) (string, bool, error) { v, ok := f[name]; return v, ok, nil }
func (f fakeRun) set(name, value string) error          { f[name] = value; return nil }
func (f fakeRun) remove(name string) error              { delete(f, name); return nil }

func TestStartWithWindowsWritesTheQuotedExeAndRemovesIt(t *testing.T) {
	run := fakeRun{}
	c := &Control{Exe: `C:\Program Files\reflbo-stats\reflbo-stats.exe`, reg: run}
	if on, _ := c.Autostart(); on {
		t.Fatal("on before it was turned on")
	}
	if err := c.SetAutostart(true); err != nil {
		t.Fatal(err)
	}
	if run["reflbo-stats"] != `"C:\Program Files\reflbo-stats\reflbo-stats.exe"` {
		t.Fatalf("the Run key holds %q", run["reflbo-stats"])
	}
	if on, _ := c.Autostart(); !on {
		t.Fatal("off after it was turned on")
	}
	if err := c.SetAutostart(false); err != nil {
		t.Fatal(err)
	}
	if _, ok := run["reflbo-stats"]; ok {
		t.Fatal("the Run key stayed")
	}
}

func TestAnotherCopyInTheRunKeyCountsAsOff(t *testing.T) {
	run := fakeRun{"reflbo-stats": `"D:\old\reflbo-stats.exe"`}
	c := &Control{Exe: `C:\Tools\reflbo-stats.exe`, reg: run}
	if on, _ := c.Autostart(); on {
		t.Fatal("another copy's entry counted as this one's")
	}
}
```


Run: `go test ./internal/autostart/`

Expected: the build fails.

```text
FAIL	github.com/Vybo/reflbo-stats-win/internal/autostart [build failed]
FAIL
# github.com/Vybo/reflbo-stats-win/internal/autostart [github.com/Vybo/reflbo-stats-win/internal/autostart.test]
internal/autostart/autostart_test.go:13:8: undefined: Control
internal/autostart/autostart_test.go:36:8: undefined: Control
```

`internal/autostart/autostart.go`:

```go
// Package autostart is Start with Windows (spec §8): the per-user Run key, so no administrator rights are needed.
package autostart

import "strings"

const valueName = "reflbo-stats"

// Control starts the exe at path after the owner logs in, or not.
type Control struct {
	Exe string // the exe's full path
	reg runStore
}

// runStore is the Run key: the real one on Windows, none elsewhere.
type runStore interface {
	get(name string) (string, bool, error)
	set(name, value string) error
	remove(name string) error
}

func New(exe string) *Control {
	return &Control{Exe: exe, reg: runKey()}
}

// command is what the Run key holds: the exe quoted, as Windows wants a path with spaces.
func (c *Control) command() string { return `"` + c.Exe + `"` }

// Autostart: the Run key names this exe. One naming another copy of the app counts as off, so turning it on
// points it here.
func (c *Control) Autostart() (bool, error) {
	v, ok, err := c.reg.get(valueName)
	if err != nil || !ok {
		return false, err
	}
	return strings.EqualFold(strings.Trim(v, `"`), c.Exe), nil
}

func (c *Control) SetAutostart(on bool) error {
	if on {
		return c.reg.set(valueName, c.command())
	}
	return c.reg.remove(valueName)
}
```


`internal/autostart/run_windows.go`:

```go
//go:build windows

package autostart

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

const runPath = `Software\Microsoft\Windows\CurrentVersion\Run`

type runKeyReg struct{}

func runKey() runStore { return runKeyReg{} }

func (runKeyReg) get(name string) (string, bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	return v, err == nil, err
}

func (runKeyReg) set(name, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

func (runKeyReg) remove(name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
```


`internal/autostart/run_other.go`:

```go
//go:build !windows

package autostart

import "errors"

// Elsewhere there is no Run key: Start with Windows is off and can't be turned on.
type noRunKey struct{}

func runKey() runStore { return noRunKey{} }

func (noRunKey) get(string) (string, bool, error) { return "", false, nil }
func (noRunKey) set(string, string) error         { return errors.New("Start with Windows needs Windows") }
func (noRunKey) remove(string) error              { return nil }
```


- [ ] **Step 2: The icons, test first.** A 32 px panel with bars, drawn by code: a mark while HWiNFO isn't "ok", a dot while a board is connected. PNG for macOS; an ICO with the PNG inside for Windows, which reads that since Vista.

`internal/icon/icon_test.go`:

```go
package icon

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"
)

func TestEachStateIsA32pxPNGAndTheyDiffer(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range []State{Normal, Warning, Connected} {
		b := PNG(s)
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%v: %v", s, err)
		}
		if r := img.Bounds(); r.Dx() != 32 || r.Dy() != 32 {
			t.Fatalf("%v: %v", s, r)
		}
		if seen[string(b)] {
			t.Fatalf("%v looks like another state", s)
		}
		seen[string(b)] = true
	}
}

func TestTheICOWrapsThePNGAsWindowsReadsIt(t *testing.T) {
	ico := ICO(Normal)
	le := binary.LittleEndian
	if le.Uint16(ico[0:]) != 0 || le.Uint16(ico[2:]) != 1 || le.Uint16(ico[4:]) != 1 {
		t.Fatalf("header % x", ico[:6])
	}
	if ico[6] != 32 || ico[7] != 32 || le.Uint16(ico[10:]) != 1 || le.Uint16(ico[12:]) != 32 {
		t.Fatalf("entry % x", ico[6:22])
	}
	size, off := le.Uint32(ico[14:]), le.Uint32(ico[18:])
	payload := PNG(Normal)
	if int(size) != len(payload) || off != 22 || !bytes.Equal(ico[22:], payload) {
		t.Fatalf("size %d off %d", size, off)
	}
}
```


`internal/icon/icon.go`:

```go
// Package icon draws the tray's icon (spec §8): a panel with bars, a warning mark while HWiNFO isn't "ok", a dot
// while a board is connected. PNG for macOS's tray, ICO (a PNG inside) for Windows'.
package icon

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
)

type State int

const (
	Normal State = iota
	Warning
	Connected
)

var (
	panel = color.RGBA{0x2f, 0x5d, 0x50, 0xff}
	ink   = color.RGBA{0xf4, 0xf5, 0xf2, 0xff}
	amber = color.RGBA{0xf2, 0xa9, 0x00, 0xff}
	green = color.RGBA{0x3d, 0xd6, 0x7a, 0xff}
)

func fill(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func disc(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r {
				img.SetRGBA(x, y, c)
			}
		}
	}
}

func draw(s State) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	fill(img, 2, 4, 30, 28, panel) // the panel, its corners left clear
	for x := 2; x < 30; x += 27 {
		img.SetRGBA(x, 4, color.RGBA{})
		img.SetRGBA(x, 27, color.RGBA{})
	}
	for i, h := range []int{6, 12, 9, 16} { // bars, as a chart
		x := 6 + i*5
		fill(img, x, 24-h, x+3, 24, ink)
	}
	switch s {
	case Warning:
		disc(img, 26, 6, 5, amber)
		fill(img, 25, 3, 27, 7, panel)
		fill(img, 25, 8, 27, 10, panel)
	case Connected:
		disc(img, 26, 26, 5, green)
	}
	return img
}

func PNG(s State) []byte {
	var b bytes.Buffer
	png.Encode(&b, draw(s))
	return b.Bytes()
}

// ICO wraps the PNG in an icon file of one image, which Windows reads since Vista.
func ICO(s State) []byte {
	payload := PNG(s)
	var b bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&b, le, [3]uint16{0, 1, 1}) // reserved, type icon, one image
	b.Write([]byte{32, 32, 0, 0})            // width, height, no palette, reserved
	binary.Write(&b, le, [2]uint16{1, 32})   // planes, bits a pixel
	binary.Write(&b, le, [2]uint32{uint32(len(payload)), 22})
	b.Write(payload)
	return b.Bytes()
}
```


- [ ] **Step 3: The app's failing tests.** The whole app, with the demo source on free ports:
  - the page's state and a board's stream with the token;
  - a second start finding the first;
  - Quit closing a board at once;
  - a missing config written with its defaults;
  - the addresses without loopback;
  - a taken LAN port keeping the page, and a new port opening for a board.

`internal/app/app_test.go`:

```go
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
```


Run: `go test ./internal/app/`

Expected: the build fails.

```text
FAIL	github.com/Vybo/reflbo-stats-win/internal/app [build failed]
FAIL
# github.com/Vybo/reflbo-stats-win/internal/app [github.com/Vybo/reflbo-stats-win/internal/app.test]
internal/app/app_test.go:36:30: undefined: App
internal/app/app_test.go:45:12: undefined: Start
internal/app/app_test.go:45:40: undefined: Options
internal/app/app_test.go:94:12: undefined: Start
internal/app/app_test.go:94:40: undefined: Options
internal/app/app_test.go:95:21: undefined: ErrRunning
internal/app/app_test.go:134:14: undefined: loadConfig
internal/app/app_test.go:145:20: undefined: Addresses
internal/app/app_test.go:164:12: undefined: Start
internal/app/app_test.go:164:40: undefined: Options
internal/app/app_test.go:164:40: too many errors
```

- [ ] **Step 4: Write the app.** Windows' "address in use" is `WSAEADDRINUSE`, which `syscall.EADDRINUSE` doesn't match there (AGENTS gotcha 3).

`internal/app/app.go`:

```go
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
```


`internal/app/errno_windows.go`:

```go
//go:build windows

package app

import "golang.org/x/sys/windows"

// wsaeaddrinuse: Windows' "address in use", which syscall.EADDRINUSE doesn't match there.
var wsaeaddrinuse error = windows.WSAEADDRINUSE
```


`internal/app/errno_other.go`:

```go
//go:build !windows

package app

import "syscall"

var wsaeaddrinuse error = syscall.EADDRINUSE
```


- [ ] **Step 5: The command and the tray.**

```bash
go get fyne.io/systray@v1.12.2
```

`cmd/reflbo-stats/main.go`:

```go
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
		w = io.MultiWriter(os.Stderr, f)
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
```


`cmd/reflbo-stats/tray.go`:

```go
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
```


`cmd/reflbo-stats/browser.go`:

```go
package main

import (
	"os/exec"
	"runtime"
)

// openBrowser opens a URL in the default browser.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}
```


- [ ] **Step 6: The page at a desktop's width.** A grid child sizes itself to its widest table unless told not to, which pushed the values and the row buttons past the window's edge.

`web/style.css`:

```diff
--- a/web/style.css
+++ b/web/style.css
@@ -11,7 +11,7 @@
 body { margin: 0; background: var(--bg); color: var(--fg); }
 header { display: flex; align-items: center; gap: 12px; padding: 14px 20px; border-bottom: 1px solid var(--line); }
 header h1 { font-size: 18px; margin: 0; }
-main { max-width: 920px; margin: 0 auto; padding: 16px 20px 48px; display: grid; gap: 16px; }
+main { max-width: 1040px; margin: 0 auto; padding: 16px 20px 48px; display: grid; grid-template-columns: minmax(0, 1fr); gap: 16px; }
 section { background: var(--card); border: 1px solid var(--line); border-radius: 8px; padding: 14px 16px; }
 h2 { font-size: 15px; margin: 0 0 10px; }
 .muted { color: var(--muted); }
@@ -30,7 +30,11 @@ summary { cursor: pointer; padding: 4px 0; }
 .value { font-variant-numeric: tabular-nums; color: var(--muted); }
 table { width: 100%; border-collapse: collapse; }
 th, td { text-align: left; padding: 4px 6px; border-bottom: 1px solid var(--line); vertical-align: middle; }
-td input[type=text] { width: 9em; } td input[type=number] { width: 5.5em; }
+td { white-space: nowrap; }
+td input[aria-label=id] { width: 10em; } td input[aria-label=label] { width: 11em; }
+td input[aria-label=unit] { width: 4em; } td input[aria-label=scale] { width: 5.5em; }
+td input[aria-label=min], td input[aria-label=max] { width: 4.5em; }
+td button { padding: 2px 7px; }
 .row { display: flex; flex-wrap: wrap; gap: 8px; align-items: center; }
 code { font: 13px ui-monospace, Menlo, Consolas, monospace; }
 .scroll { overflow-x: auto; }
```


- [ ] **Step 7: Run everything, build the exe, and commit.**

Run: `gofmt -l . ; go vet ./... && GOOS=windows go vet ./... && go test -race -count=1 ./... && node --test test/web/app.test.mjs 2>&1 | grep -E '^# (pass|fail)' && GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags '-H windowsgui -s -w' -o dist/reflbo-stats.exe ./cmd/reflbo-stats && file dist/reflbo-stats.exe`

Expected: every package passes, the page's tests pass, and the exe is a Windows GUI binary.

```text
?   	github.com/Vybo/reflbo-stats-win/cmd/reflbo-stats	[no test files]
ok  	github.com/Vybo/reflbo-stats-win/internal/app	Ns
ok  	github.com/Vybo/reflbo-stats-win/internal/autostart	Ns
ok  	github.com/Vybo/reflbo-stats-win/internal/config	Ns
ok  	github.com/Vybo/reflbo-stats-win/internal/hwinfo	Ns
?   	github.com/Vybo/reflbo-stats-win/internal/hwinfo/hwinfotest	[no test files]
ok  	github.com/Vybo/reflbo-stats-win/internal/icon	Ns
ok  	github.com/Vybo/reflbo-stats-win/internal/server	Ns
ok  	github.com/Vybo/reflbo-stats-win/internal/source	Ns
?   	github.com/Vybo/reflbo-stats-win/web	[no test files]
# pass 14
# fail 0
dist/reflbo-stats.exe: PE32+ executable (GUI) x86-64, for MS Windows
```

`go.mod`:

```diff
--- a/go.mod
+++ b/go.mod
@@ -4,4 +4,9 @@ go 1.26.0
 
 require golang.org/x/sys v0.49.0
 
-require github.com/coder/websocket v1.8.15
+require (
+	fyne.io/systray v1.12.2
+	github.com/coder/websocket v1.8.15
+)
+
+require github.com/godbus/dbus/v5 v5.1.0 // indirect
```


```bash
go mod tidy
git add -A
git commit -m "feat: the app as a whole: both listeners, the tray, Start with Windows, one instance (spec §3, §8)"
```

### Task 7: The docs as built

**Files:**
- Create: `README.md` (in place of the first commit's title), `AGENTS.md`, `scripts/build.sh`, `docs/images/settings.png`
- Modify: `docs/spec.md` (r3, as built)

**Interfaces:** none; the docs describe Tasks 1–6.

- [ ] **Step 1: The build script.**

`scripts/build.sh`:

```sh
#!/bin/sh
# Builds dist/reflbo-stats.exe for Windows x64 from any system with Go 1.26 or later (spec §3).
#   scripts/build.sh [version]     # the version defaults to git describe
set -eu
cd "$(dirname "$0")/.."
version="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
mkdir -p dist
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags "-H windowsgui -s -w -X main.version=$version" -o dist/reflbo-stats.exe ./cmd/reflbo-stats
echo "dist/reflbo-stats.exe ($version, $(wc -c < dist/reflbo-stats.exe | tr -d ' ') bytes)"
```


Run: `chmod +x scripts/build.sh && scripts/build.sh 0.1.0`

Expected: the exe, about 8 MB.

```text
dist/reflbo-stats.exe (0.1.0, 8139776 bytes)
```

- [ ] **Step 2: The page's screenshot.** Run the demo with a fresh config, pick seven readings through the API (the PC layout's defaults: CPU and GPU load and temperature, RAM, VRAM and the network), and screenshot the page in headless Chrome:

```bash
d=$(mktemp -d) && go build -o dist/reflbo-stats-mac ./cmd/reflbo-stats
./dist/reflbo-stats-mac -demo -no-tray -config $d/config.json & sleep 2
curl -s -X PUT -H 'Content-Type: application/json' http://127.0.0.1:47124/api/readings -d '[
 {"key":"demo:cpu:load","id":"cpu_load","label":"CPU load","unit":"%","decimals":0,"min":0,"max":100},
 {"key":"demo:cpu:temp","id":"cpu_temp","label":"CPU","unit":"°C","decimals":0},
 {"key":"demo:gpu:load","id":"gpu_load","label":"GPU load","unit":"%","decimals":0,"min":0,"max":100},
 {"key":"demo:gpu:temp","id":"gpu_temp","label":"GPU","unit":"°C","decimals":0},
 {"key":"demo:mem:used","id":"ram_used","label":"RAM","unit":"GB","scale":0.0009765625,"decimals":1,"min":0,"max":64},
 {"key":"demo:gpu:vram","id":"vram_used","label":"VRAM","unit":"GB","scale":0.0009765625,"decimals":1,"min":0,"max":12},
 {"key":"demo:net:down","id":"net_down","label":"Network","unit":"MB/s","scale":0.0009765625,"decimals":1}]' -o /dev/null -w '%{http_code}\n'
"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless=new --hide-scrollbars --window-size=1100,1500 \
  --virtual-time-budget=4000 --screenshot=docs/images/settings.png http://127.0.0.1:47124/
kill %1
```

Expected: `200`, and a screenshot showing the seven readings with their live values (22.6 GB and the like) above HWiNFO's demo sensors. Look at it.

- [ ] **Step 3: The README**: what the app is for, what it needs (HWiNFO Pro with Shared Memory Support), the first start (SmartScreen, the firewall's private networks, the page, the token for reflbo's PC page), the tray, the readings' fields, what to do for each state the page reports, and the commands for developers.

`README.md`:

````markdown
# reflbo-stats

A small Windows tray app that serves this PC's sensors, as [HWiNFO](https://www.hwinfo.com) measures them, to [reflbo](https://github.com/Vybo/reflbo) desk displays on your home network. While the PC is on, a reflbo board can show its CPU and GPU load and temperatures, memory and network as numbers, history charts, bars and gauges. When the PC sleeps, the board goes back to its usual dashboards.

<p><img src="docs/images/settings.png" width="560" alt="The settings page: HWiNFO's state, the readings the boards get with their ids, units, scale and range, and HWiNFO's readings to pick from"></p>

## What you need

- Windows 10 or 11, 64-bit.
- **HWiNFO Pro**, running with **Shared Memory Support** on (HWiNFO ▸ Settings ▸ Shared Memory Support). Sensors-only mode is enough. The free HWiNFO switches shared memory off after 12 hours.
- A reflbo board with firmware M7 or later, on the same network.

## Install and first start

1. Download `reflbo-stats.exe` and put it where it can stay, for example `C:\Tools\reflbo-stats\`.
2. Run it. The file isn't signed, so Windows SmartScreen may warn the first time: choose **More info ▸ Run anyway**.
3. Windows' firewall asks whether reflbo-stats may use the network. Allow it for **private networks**: the boards connect to it. If Windows has your home network marked Public, mark it Private (Settings ▸ Network & internet ▸ your network ▸ Private), or allow the app for public networks too.
4. The settings page opens in your browser (`http://127.0.0.1:47124/`). Tick the readings you want the boards to have, adjust them, and **Save**.
5. On reflbo's web page, open **PC** and enter the address, port and token the settings page shows under **Connection**.

The board looks for the PC at each sync. When it finds it, it can switch to sync mode Always on and to its PC preset; both are options on its PC page.

## The tray

- **Left-click:** opens the settings page.
- **Right-click:**
  - **Open settings**;
  - **Start with Windows**, which starts the app after you log in (off at first);
  - **Quit**, which tells the boards at once, so they return to their dashboards without waiting.
- **The icon:** a mark appears while HWiNFO isn't delivering readings, and a dot while a board is connected.

## The readings

Each picked reading has:

- **Id:** how the board's presets name it, as `pc.<id>`: a-z, 0-9 and _, up to 15 characters. Renaming it means updating the presets that use it.
- **Label** and **unit:** what the board shows, up to 23 and 7 bytes.
- **Scale:** multiplies HWiNFO's value. `1/1024` turns MB into GB, and the unit follows it.
- **Decimals:** 0 to 3.
- **Min** and **max:** the usual range, where the board's bars, rings and gauges start. A % runs 0–100 unless you change it; leave both empty for an automatic chart.

Up to 32 readings. The board's PC preset fills its charts and bars from the first ones in the list, so put the ones you want there at the top.

## When something isn't right

| The page says | What to do |
|---|---|
| HWiNFO isn't running | Start HWiNFO with Shared Memory Support on |
| Shared Memory Support is off | Turn it on in HWiNFO's Settings |
| HWiNFO has stopped updating | HWiNFO is paused, or the free version's 12 hours are up |
| No board is connected | Check the address, port and token on reflbo's PC page, and that the firewall allows reflbo-stats on private networks |
| The boards can't reach this PC | Another program holds the LAN port: choose another under App and Apply, then enter it on reflbo's PC page |

The settings and a log live in `%APPDATA%\reflbo-stats\` (`config.json`, `log.txt`). **New token**, under Connection, disconnects every board until it gets the new one.

## For developers

```sh
go test ./... && node --test test/web/app.test.mjs   # on any system
go run ./cmd/reflbo-stats -demo -no-tray             # a plausible PC, served on this machine
scripts/build.sh                                     # dist/reflbo-stats.exe, cross-compiled
reflbo-stats.exe -dump hwinfo.bin                    # on the PC: HWiNFO's shared memory for the tests (log.txt says how it went)
```

[AGENTS.md](AGENTS.md) has the details; [docs/spec.md](docs/spec.md) the design. The protocol is reflbo's: its design spec, §12a.

## Licence

Apache-2.0. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
````


- [ ] **Step 4: AGENTS.md**: the layout, the commands, the rules and the gotchas.

`AGENTS.md`:

````markdown
# AGENTS.md — reflbo-stats

Guide for agents (and humans) working in this repository. Keep it current in the same commit as any new command, package or rule.

> **Scope.** A standalone Go app for Windows, the PC half of reflbo's M7 (reflbo D44). Instructions inherited from parent directories about iOS, WeConnect-iOS, Jira or PR templates do not apply here.

## What it is

A tray app that reads HWiNFO Pro's shared memory and serves the readings the owner picks to reflbo boards on the LAN, over WebSocket with a token. The design is [docs/spec.md](docs/spec.md). The protocol is reflbo's, its design spec §12a (`~/reflbo/docs/specs/2026-09-25-firmware-design.md`); if the two disagree on the protocol, reflbo's wins.

## Layout

```
cmd/reflbo-stats/      main: flags (-demo, -dump, -no-tray, -config, -version), the tray, the browser
internal/app/          start and stop: the config, the source, the hub, both listeners; one instance
internal/autostart/    Start with Windows: the per-user Run key (none elsewhere)
internal/config/       config.json: picked readings, token, ports; ids from labels; atomic save with .bak
internal/hwinfo/       HWiNFO's shared memory: Parse (pure Go) and ReadMapping (Windows)
internal/hwinfo/hwinfotest/  builds mappings for tests
internal/icon/         the tray's icons, drawn by code (PNG; ICO for Windows)
internal/server/       the hub, the LAN API (/v1/info, /v1/stream) and the settings page's API
internal/source/       HWiNFO's source and the demo's; the catalogue and the values boards get
web/                   the settings page: plain HTML, CSS and JS, embedded
test/web/              the page's tests (Node, no packages)
scripts/build.sh       dist/reflbo-stats.exe, cross-compiled
```

## Commands

```sh
go vet ./... && GOOS=windows go vet ./...       # both systems: Windows-only files are compiled only there
go test -race ./...                             # everything but the page, on any system
node --test test/web/app.test.mjs               # the page against a fake app
go run ./cmd/reflbo-stats -demo -no-tray -config /tmp/rs/config.json   # serves a demo PC here: page on :47124
scripts/build.sh [version]                      # the Windows exe
gofmt -l .                                      # must print nothing
```

Go 1.26 or later, as `golang.org/x/sys` v0.49 needs it (`brew install go` on the owner's Mac). Dependencies: `fyne.io/systray`, `github.com/coder/websocket`, `golang.org/x/sys`; add no others without the owner.

## Rules

- **Verify at the right level.** Logic in Go tests on the Mac; the page in Node; the exe cross-compiled; a board end to end against `-demo` on the Mac; only HWiNFO itself, the tray and the firewall need the owner's PC (give an exact checklist).
- **Windows-only code stays thin** and behind build tags (`_windows.go` / `_other.go`), and `GOOS=windows go vet ./...` passes.
- **The token is a secret.** Never log it, never commit a config.json. Dumps (`-dump`) hold sensor names and values only; keep personal ones out of git (`testdata/*.local.bin` is ignored).
- **The settings page answers on 127.0.0.1 only**, checks Host and Origin and takes changes as JSON only. Keep it so.
- **No AI or assistant attribution anywhere:** commits, code comments, docs.
- Conventional Commits, imperative mood. The owner allows pushing to `origin`; never force-push `main`.
- Code from hwinfo-streamdeck (GPL-3.0) is never copied; it is a reference for what HWiNFO offers.

## Gotchas

1. **HWiNFO's layout comes from its published header**, read by the sizes in the mapping's own header; an unknown version is refused, not guessed. The owner's dump confirms it on the PC.
2. **`go vet` flags a uintptr turned into a pointer.** The mapped view becomes a slice through a slice header (`mapping_windows.go`).
3. **Windows' "address in use" is WSAEADDRINUSE**, not syscall.EADDRINUSE; `internal/app` checks both for its one-instance rule.
4. **`node --test test/web/`** treats the directory as one file in Node 22: name the file.
5. **A Windows mutex belongs to the thread that takes it.** `ReadMapping` locks its goroutine to one OS thread until it lets HWiNFO's mutex go; otherwise the release fails on another thread and the mutex stays taken.
6. **JSON has no NaN or infinity.** A non-finite double makes `json.Marshal` fail, which would close every board; `source.finite` turns them into `null` before anything leaves, including a value a scale overflows.
````


- [ ] **Step 5: The spec as built (r3)**: the known layouts and their refusals, staleness by HWiNFO's polling period, non-finite numbers, the mutex's thread (§4); the defaults the app fills in (§5); the closing reasons, the source's pace, a taken LAN port (§6); the page and the tray as built (§7, §8); the log (§9); the review's minors as open items (§12).

`docs/spec.md`:

````diff
--- a/docs/spec.md
+++ b/docs/spec.md
@@ -1,7 +1,7 @@
 # reflbo-stats: design spec
 
 - **Date:** 2026-10-19
-- **Status:** Draft for the owner's review (r1)
+- **Status:** As built (r3, after the branch's review); the owner's checks on the PC wait (§10)
 - **Covers:** the Windows tray app that serves a PC's HWiNFO sensors to [reflbo](https://github.com/Vybo/reflbo) boards on the LAN
 - **Related:** reflbo's design spec, `docs/specs/2026-09-25-firmware-design.md` in the reflbo repository: §12a (the protocol and the board's side), decision D44 (§1.2), milestone M7 (§18). Where the two disagree on the protocol, reflbo's §12a wins; fix this file.
 
@@ -45,16 +45,18 @@ testdata/           HWiNFO dumps made with -dump on the owner's PC
 ## 4. Reading HWiNFO
 
 - **Where:** the file mapping `Global\HWiNFO_SENS_SM2`, opened read-only (`OpenFileMapping`, `MapViewOfFile`), as HWiNFO's published shared-memory header (`hwisenssm2.h`) describes it. The header is signature `HWiS`, version, revision, poll time, and the offsets, sizes and counts of the sensor and reading sections. Each reading has its type, sensor index, reading id, labels, unit, and value, minimum, maximum and average.
-- **Element sizes:** the reader takes them from the header, so a newer HWiNFO with longer elements still reads. It refuses a version it doesn't know, or sizes smaller than the fields it reads, and says so on the page.
+- **Element sizes:** the reader takes them from the header, so a newer HWiNFO with longer elements still reads. It refuses a version it doesn't know (as built it knows 1 and 2), or sizes smaller than the fields it reads, and says so on the page. The boards then see "not running" (the protocol's four states have no fifth), while the page shows the reason.
 - **States:**
   - **not running:** no mapping.
   - **shm off:** HWiNFO runs, the mapping is missing: Shared Memory Support is off in HWiNFO's settings.
-  - **stale:** the poll time hasn't moved for 5 s.
+  - **stale:** the poll time hasn't moved for three of HWiNFO's polling periods (the header's, offset 44), and never under 5 s, as the poll time counts whole seconds.
   - **ok:** otherwise.
   - The board gets them in `/v1/info` and in `status` messages.
 - **A reading's key:** sensor id, sensor instance and reading id, which stay the same across HWiNFO's restarts. The page shows HWiNFO's user labels where the owner renamed them in HWiNFO, else its original ones.
 - **Text:** labels are read as UTF-8 where HWiNFO gives them (its `utf` fields), else from its ANSI ones in the system code page.
 - **`-dump <file>`** writes the mapping as it is once, for tests. A dump holds sensor names and values only.
+- **Reading it** (as built): under HWiNFO's mutex (`Global\HWiNFO_SM2_MUTEX`, 50 ms at most; an abandoned one counts as taken) when it has one, so a poll in progress isn't read half-written, with the goroutine locked to its OS thread meanwhile, as a Windows mutex belongs to its thread; "shm off" against "not running" by whether an `hwinfo*.exe` process runs.
+- **Numbers:** a value, minimum or maximum that isn't finite (HWiNFO's NaN, an infinity) is no value (`null`), as JSON can't carry it; so is a value a scale overflows.
 
 ## 5. The picked readings
 
@@ -69,6 +71,8 @@ Each reading the owner ticks gets:
 
 At most 32 readings. Their order on the page is the catalogue's order, and the board's PC preset fills its slots from the first ones.
 
+As built: a newly ticked reading goes to the app with its key alone, and the app fills in the rest on Save, so its rules live in one place: the label and unit from HWiNFO (cut at a character), the id from the label, the scale 1, decimals 0 for %, MHz, MB, RPM, KB/s and MB/s, 3 for V and 1 for the rest, and 0–100 for a %. The scale is typed as a number or a fraction (`1/1024`) and shown back as one.
+
 ## 6. The LAN API (reflbo §12a)
 
 - **Port** 47123 by default, on all interfaces, for `/v1/*` only. Every request needs `Authorization: Bearer <token>`, otherwise 401.
@@ -79,7 +83,10 @@ At most 32 readings. Their order on the page is the catalogue's order, and the b
   - It sends the `catalog` again after any change on the page, and a `status` when HWiNFO's state changes.
   - It pings every 5 s.
   - `null` stands for a reading HWiNFO doesn't deliver now (stale, gone, or HWiNFO not running).
-- **Limits:** frames of at most 8 KB each way; a board that sends more, or JSON deeper than 4 levels, is closed. At most 8 boards at once.
+- **Limits:** frames of at most 8 KB each way; a board that sends more, or JSON deeper than 4 levels, is closed. At most 8 boards at once; a ninth gets 503.
+- **Closing** (as built), with the reason a board sees: "no hello" after 5 s without one, "protocol N", "too deep" and "new token" (policy violation, 1008), a frame over 8 KB (1009), and Quit (going away, 1001).
+- **The source** is read at the fastest interval a connected board asked for, never below 250 ms, and once a second with no board.
+- **A LAN port the app can't open** (another program holds it) doesn't stop it: the settings page and the tray run, the page says why under Connection (and opens by itself at a start), and Apply on a new port opens it at once.
 - **Token:** 128 random bits, shown as 26 base32 characters in groups. "New token" on the page closes every board's connection.
 
 ## 7. The settings page
@@ -93,7 +100,12 @@ At most 32 readings. Their order on the page is the catalogue's order, and the b
     - the PC's address(es) and port, to enter on reflbo's PC page;
     - the token, with Copy and New token;
     - the LAN port (a restart applies it).
-  - **App:** Start with Windows, the version, and the config file's path.
+  - **App:** Start with Windows, the LAN port (Apply opens it at once), the version, and the config file's path.
+- **As built:**
+  - the live values refresh every second without redrawing the fields being edited; an edit updates Save and Undo in place, so the focus stays and the first click saves; the search redraws the list under its box, and a group the owner collapsed stays collapsed;
+  - New token takes a second click to confirm (no `confirm()`, so a driven browser doesn't block);
+  - Save and Undo changes work on the whole list;
+  - a refusal shows the app's reason and keeps the edits.
 - **Windows firewall:** the first start asks to allow the app; private networks are enough. The page says so if no board has connected.
 
 ## 8. The tray
@@ -105,7 +117,9 @@ At most 32 readings. Their order on the page is the catalogue's order, and the b
   - **Quit**, which closes the boards' connections cleanly, so they end their sessions at once rather than after 15 s.
 - **Icon:** shows the state: normal, a warning mark while HWiNFO isn't "ok", and a dot while a board is connected.
 - **Auto-start:** off by default. When on, it is the per-user Run key (`HKCU\Software\Microsoft\Windows\CurrentVersion\Run`) with the exe's path, so no administrator rights are needed. Turning it off removes the key.
-- **One instance:** a second start opens the first one's page and exits.
+- **One instance:** a second start opens the first one's page and exits. As built, the settings page's port tells: a second app can't listen on it.
+- **The first start**, with no reading picked, opens the settings page.
+- **The tooltip** reads HWiNFO's state and the number of boards; the menu's Start with Windows follows the page's checkbox within 2 s.
 
 ## 9. Configuration
 
@@ -123,7 +137,7 @@ At most 32 readings. Their order on the page is the catalogue's order, and the b
 }
 ```
 
-A reading whose key HWiNFO no longer has (another CPU, a removed drive) stays in the list, marked missing, and is sent as `null`. Auto-start lives in the Run key, not in the file.
+A reading whose key HWiNFO no longer has (another CPU, a removed drive) stays in the list, marked missing, and is sent as `null`. Auto-start lives in the Run key, not in the file. A missing or broken file and `.bak` give the defaults, written at once. The log is `log.txt` beside it, started afresh past 1 MB; the exe has no console.
 
 ## 10. Tests and checks
 
@@ -150,14 +164,35 @@ A reading whose key HWiNFO no longer has (another CPU, a removed drive) stays in
 | HWiNFO changes its shared memory | The header's sizes are honoured; an unknown version is refused and named on the page |
 | The unsigned exe trips SmartScreen; an antivirus may flag an exe that reads another process's shared memory | Warn in the README; build reproducibly from the source |
 | The token travels in plain HTTP on the LAN | LAN only; New token on the page |
-| A port is taken | The page and the log say which; both ports are configurable |
-
-## 12. Licence
+| A port is taken | The LAN port: the page and the log say which, and another opens at once; the settings page's: a second copy takes it for the first one running, and it is set in config.json |
+
+## 12. Open items
+
+Minors from the branch's review (2026-10-19), to fix where later work touches the code:
+- Nothing keeps the app's own frames under 8 KB: labels full of `<`, `>` or `&` are escaped to 6 bytes each (`SetEscapeHTML(false)` and a size check on Save).
+- A new token or Quit closes the boards one after another, 5 s each for one that doesn't answer; the LAN listener should stop first, and the closes run at once.
+- The ping waits in the write loop, delaying the readings by a round trip every 5 s.
+- A hello without `interval_ms` gets 250 ms, not 1 s; `1000.0` doesn't parse.
+- The source's loop and each board's ticker aren't in step, so a value can be an interval old or repeat; a board before its hello slows the loop to 10 s.
+- A fifth interval change in a row is dropped (a queue of four).
+- Any settings change resends an unchanged catalogue.
+- The config file is two renames, not one replace (`ReplaceFileW`), and Defender's sharing violations aren't retried; the page's API doesn't serialise its read-modify-write of it.
+- ANSI text reads as Latin-1, not the system code page (cp1250 letters); a UTF-8 field cut mid-character falls back whole.
+- The scale field shows bad input as NaN and refuses a decimal comma; `maxlength` counts characters where the app counts bytes.
+- The Origin check takes any loopback port; an oversized body gets "send a list of readings", not 413.
+- Every Windows session shares loopback: another logged-in user's browser reaches this user's page.
+- Start with Windows stays ticked when Task Manager has disabled the entry (`StartupApproved\Run`).
+- The addresses include virtual adapters (WSL, Hyper-V, VPN); a board before its hello lists "0 ms".
+- No test covers a half-open or slow board, or the page's focus (the fake DOM has none).
+
+## 13. Licence
 
 Apache-2.0, as reflbo. hwinfo-streamdeck (GPL-3.0) is a reference for what HWiNFO offers, never a source of code.
 
-## 13. Revision history
+## 14. Revision history
 
 | Rev | Date | Change |
 |---|---|---|
 | r1 | 2026-10-19 | First draft from the owner's brainstorming answers (reflbo D44) |
+| r2 | 2026-10-19 | As built on `plan/m7`: the known layouts and their refusals (§4), defaults filled by the app (§5), closing reasons and the source's pace (§6), the page and the tray as built (§7, §8), the log (§9) |
+| r3 | 2026-10-19 | The branch's review: stale by HWiNFO's polling period, non-finite numbers as none, the mutex's thread (§4); a taken LAN port (§6, §11); the page's edits in place (§7); the open items (§12) |
````


- [ ] **Step 6: Commit, push, and hand over.**

```bash
git add -A
git commit -m "docs: the README, AGENTS.md, the build script and the spec as built (r3)"
git push origin main
```

Then give the owner the exe (`dist/reflbo-stats.exe`) and the checklist under Owner checks.

## Owner checks

On the PC, with HWiNFO Pro running in sensors-only mode with Shared Memory Support on:

1. **It starts:** `reflbo-stats.exe` runs (SmartScreen: More info ▸ Run anyway); the firewall asks, and private networks are allowed; the page opens by itself.
2. **HWiNFO is found:** the page says "HWiNFO is running, and its readings come in." and lists your sensors with live values; the tray icon shows no mark.
3. **The picker:** tick CPU and GPU load and temperature, RAM (Physical Memory Used, scale 1/1024, unit GB), VRAM and the network; Save says "Saved."; the values column shows them as a board gets them.
4. **The tray:** left-click opens the page; right-click shows Open settings, Start with Windows and Quit.
5. **Start with Windows:** tick it, log out and in: the app runs (its icon is there), and the page's box is ticked too.
6. **Quit:** the app leaves the tray. With a board connected, the board ends its session at once.
7. **A dump for the tests:** `reflbo-stats.exe -dump %USERPROFILE%\Desktop\hwinfo.bin` (quit the app first); `log.txt` in `%APPDATA%\reflbo-stats\` says how many bytes it wrote. Send the file: it holds sensor names and values only.
