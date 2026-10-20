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
