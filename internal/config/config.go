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
