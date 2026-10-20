//go:build !windows

package autostart

import "errors"

// Elsewhere there is no Run key: Start with Windows is off and can't be turned on.
type noRunKey struct{}

func runKey() runStore { return noRunKey{} }

func (noRunKey) get(string) (string, bool, error) { return "", false, nil }
func (noRunKey) set(string, string) error         { return errors.New("Start with Windows needs Windows") }
func (noRunKey) remove(string) error              { return nil }
