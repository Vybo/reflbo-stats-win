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
