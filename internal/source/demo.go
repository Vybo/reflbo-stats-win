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
