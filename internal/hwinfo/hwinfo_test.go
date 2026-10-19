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
