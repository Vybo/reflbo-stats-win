package autostart

import "testing"

type fakeRun map[string]string

func (f fakeRun) get(name string) (string, bool, error) { v, ok := f[name]; return v, ok, nil }
func (f fakeRun) set(name, value string) error          { f[name] = value; return nil }
func (f fakeRun) remove(name string) error              { delete(f, name); return nil }

func TestStartWithWindowsWritesTheQuotedExeAndRemovesIt(t *testing.T) {
	run := fakeRun{}
	c := &Control{Exe: `C:\Program Files\reflbo-stats\reflbo-stats.exe`, reg: run}
	if on, _ := c.Autostart(); on {
		t.Fatal("on before it was turned on")
	}
	if err := c.SetAutostart(true); err != nil {
		t.Fatal(err)
	}
	if run["reflbo-stats"] != `"C:\Program Files\reflbo-stats\reflbo-stats.exe"` {
		t.Fatalf("the Run key holds %q", run["reflbo-stats"])
	}
	if on, _ := c.Autostart(); !on {
		t.Fatal("off after it was turned on")
	}
	if err := c.SetAutostart(false); err != nil {
		t.Fatal(err)
	}
	if _, ok := run["reflbo-stats"]; ok {
		t.Fatal("the Run key stayed")
	}
}

func TestAnotherCopyInTheRunKeyCountsAsOff(t *testing.T) {
	run := fakeRun{"reflbo-stats": `"D:\old\reflbo-stats.exe"`}
	c := &Control{Exe: `C:\Tools\reflbo-stats.exe`, reg: run}
	if on, _ := c.Autostart(); on {
		t.Fatal("another copy's entry counted as this one's")
	}
}
