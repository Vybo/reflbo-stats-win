package hwinfo

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"github.com/Vybo/reflbo-stats-win/internal/hwinfo/hwinfotest"
)

type fsensor = hwinfotest.Sensor
type freading = hwinfotest.Reading

func fixture(sig, version uint32, poll int64, sensorSize, readingSize int, sensors []fsensor, readings []freading) []byte {
	return hwinfotest.Build(sig, version, poll, sensorSize, readingSize, sensors, readings)
}

func sample(sensorSize, readingSize int) []byte {
	return fixture(SignatureLive, 1, 1760880000, sensorSize, readingSize,
		[]fsensor{
			{ID: 0xf0000400, Inst: 0, Orig: "CPU [#0]: AMD Ryzen 7 7800X3D", UTF8: "CPU [#0]: AMD Ryzen 7 7800X3D"},
			{ID: 0xe0002000, Inst: 1, Orig: "GPU [#0]: NVIDIA GeForce RTX 4070", User: "My GPU", UTF8: "My GPU"},
		},
		[]freading{
			{Type: uint32(TypeUsage), Sensor: 0, ID: 0x7000000, Orig: "Total CPU Usage", Unit: "%", UTFLabel: "Total CPU Usage", UTFUnit: "%", Value: 12.5, Min: 0.4, Max: 98, Avg: 10.1},
			{Type: uint32(TypeTemp), Sensor: 0, ID: 0x1000000, Orig: "CPU (Tctl/Tdie)", User: "CPU", Unit: "\xb0C", UTFLabel: "CPU", UTFUnit: "°C", Value: 54.25},
			{Type: uint32(TypeTemp), Sensor: 1, ID: 0x1000000, Orig: "GPU Temperature", Unit: "\xb0C", Value: 41},
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
	b := fixture(SignatureLive, 1, 0, sensorUTFSize, readingUTFSize, []fsensor{{ID: 1, Orig: "A"}},
		[]freading{{Sensor: 0, ID: 1, Orig: "kept"}, {Sensor: 5, ID: 2, Orig: "orphan"}})
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
