//go:build windows

package app

import "golang.org/x/sys/windows"

// wsaeaddrinuse: Windows' "address in use", which syscall.EADDRINUSE doesn't match there.
var wsaeaddrinuse error = windows.WSAEADDRINUSE
