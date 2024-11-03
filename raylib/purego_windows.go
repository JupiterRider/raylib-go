//go:build !cgo && windows
// +build !cgo,windows

package rl

import (
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
	"golang.org/x/sys/windows"

	_ "embed"
)

const (
	libname         = "raylib.dll"
	requiredVersion = "5.0"
)

//go:embed blobs/raylib.dll.gz
var compressedDll embed.FS

var wvsprintfA uintptr

func init() {
	handle, err := windows.LoadLibrary("user32.dll")
	if err == nil {
		wvsprintfA, _ = windows.GetProcAddress(handle, "wvsprintfA")
	}
}

// writeDll decompresses the raylib library and writes it into a temp directory
func writeDll() (string, error) {
	dllPath := filepath.Join(os.TempDir(), libname)

	_, err := os.Stat(dllPath)

	if os.IsNotExist(err) {
		dllFile, err := os.Create(dllPath)
		if err != nil {
			return "", err
		}
		defer dllFile.Close()

		file, err := compressedDll.Open("blobs/raylib.dll.gz")
		if err != nil {
			return "", err
		}
		defer file.Close()

		gz, err := gzip.NewReader(file)
		if err != nil {
			return "", err
		}
		defer gz.Close()

		_, err = io.Copy(dllFile, gz)
		if err != nil {
			return "", err
		}
	}

	return dllPath, nil
}

// loadLibrary loads the raylib dll and panics on error
func loadLibrary() uintptr {
	dllPath, err := writeDll()
	if err != nil {
		panic(err)
	}

	handle, err := windows.LoadLibrary(dllPath)
	if err != nil {
		panic(fmt.Errorf("cannot load library %s: %w", dllPath, err))
	}

	proc, err := windows.GetProcAddress(handle, "raylib_version")
	if err != nil {
		panic(err)
	}

	version := windows.BytePtrToString(**(***byte)(unsafe.Pointer(&proc)))
	if version != requiredVersion {
		panic(fmt.Errorf("version %s of %s doesn't match the required version %s", version, dllPath, requiredVersion))
	}

	return uintptr(handle)
}

func traceLogCallbackWrapper(fn TraceLogCallbackFun) uintptr {
	return purego.NewCallback(func(logLevel int32, text *byte, args unsafe.Pointer) uintptr {
		if wvsprintfA != 0 {
			var buffer [1024]byte // Max size is 1024 (see https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-wvsprintfa)
			_, _, errno := syscall.SyscallN(wvsprintfA, uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(text)), uintptr(args))
			if errno == 0 {
				text = &buffer[0]
			}
		}
		fn(int(logLevel), windows.BytePtrToString(text))
		return 0
	})
}
