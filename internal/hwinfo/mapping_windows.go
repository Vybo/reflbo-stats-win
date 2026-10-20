//go:build windows

package hwinfo

import (
	"errors"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procOpenFileMapping = windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenFileMappingW")

const fileMapRead = 0x0004 // FILE_MAP_READ

// ErrNoMapping: HWiNFO isn't running, or runs with Shared Memory Support off (Running tells them apart).
var ErrNoMapping = errors.New("HWiNFO's shared memory isn't there")

// ReadMapping copies the whole mapping, under HWiNFO's mutex when it has one, so a poll in progress isn't
// read half-written.
func ReadMapping() ([]byte, error) {
	// A Windows mutex belongs to the thread that takes it: the goroutine stays on one OS thread until it lets go,
	// or ReleaseMutex would fail on another one and the mutex stay taken.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	name, _ := windows.UTF16PtrFromString(MappingName)
	h, _, callErr := procOpenFileMapping.Call(fileMapRead, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		if errors.Is(callErr, windows.ERROR_FILE_NOT_FOUND) {
			return nil, ErrNoMapping
		}
		return nil, callErr
	}
	defer windows.CloseHandle(windows.Handle(h))
	addr, err := windows.MapViewOfFile(windows.Handle(h), fileMapRead, 0, 0, 0)
	if err != nil {
		return nil, err
	}
	defer windows.UnmapViewOfFile(addr)
	var info windows.MemoryBasicInformation
	if err := windows.VirtualQuery(addr, &info, unsafe.Sizeof(info)); err != nil {
		return nil, err
	}
	if mutexName, err := windows.UTF16PtrFromString(MutexName); err == nil {
		if m, err := windows.OpenMutex(windows.SYNCHRONIZE, false, mutexName); err == nil {
			defer windows.CloseHandle(m) // runs last
			ev, _ := windows.WaitForSingleObject(m, 50)
			if ev == windows.WAIT_OBJECT_0 || ev == windows.WAIT_ABANDONED { // abandoned: ours now all the same
				defer windows.ReleaseMutex(m) // runs before the close, after the copy
			}
		}
	}
	var view []byte // the mapped view as a slice, without converting the uintptr itself (go vet's unsafeptr)
	hdr := (*struct {
		data     uintptr
		len, cap int
	})(unsafe.Pointer(&view))
	hdr.data, hdr.len, hdr.cap = addr, int(info.RegionSize), int(info.RegionSize)
	return append([]byte(nil), view...), nil
}

// Running: an HWiNFO process is there (HWiNFO64.exe, HWiNFO32.exe or HWiNFO.exe).
func Running() bool {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := strings.ToLower(windows.UTF16ToString(e.ExeFile[:]))
		if strings.HasPrefix(name, "hwinfo") && strings.HasSuffix(name, ".exe") {
			return true
		}
	}
	return false
}
