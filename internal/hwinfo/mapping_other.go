//go:build !windows

package hwinfo

import "errors"

// ErrNoMapping: there is no HWiNFO outside Windows; the demo source stands in (spec §3).
var ErrNoMapping = errors.New("HWiNFO's shared memory isn't there")

func ReadMapping() ([]byte, error) { return nil, ErrNoMapping }

func Running() bool { return false }
