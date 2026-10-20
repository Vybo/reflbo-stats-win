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
