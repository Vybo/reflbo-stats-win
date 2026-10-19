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
