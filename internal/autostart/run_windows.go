//go:build windows

package autostart

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

const runPath = `Software\Microsoft\Windows\CurrentVersion\Run`

type runKeyReg struct{}

func runKey() runStore { return runKeyReg{} }

func (runKeyReg) get(name string) (string, bool, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runPath, registry.QUERY_VALUE)
	if err != nil {
		return "", false, err
	}
	defer k.Close()
	v, _, err := k.GetStringValue(name)
	if errors.Is(err, registry.ErrNotExist) {
		return "", false, nil
	}
	return v, err == nil, err
}

func (runKeyReg) set(name, value string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, value)
}

func (runKeyReg) remove(name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
