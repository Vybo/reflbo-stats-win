//go:build !windows

package app

import "syscall"

var wsaeaddrinuse error = syscall.EADDRINUSE
