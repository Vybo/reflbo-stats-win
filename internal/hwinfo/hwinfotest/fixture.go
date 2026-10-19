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
