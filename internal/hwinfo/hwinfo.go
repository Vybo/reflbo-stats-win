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
