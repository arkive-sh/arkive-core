package opaque

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

type bindings struct {
	generateSetup        func(*unsafe.Pointer, *uintptr) unsafe.Pointer
	validateSetup        func(unsafe.Pointer, uintptr) unsafe.Pointer
	registrationResponse func(unsafe.Pointer, uintptr, unsafe.Pointer, uintptr, unsafe.Pointer, uintptr, *unsafe.Pointer, *uintptr) unsafe.Pointer
	finishRegistration   func(unsafe.Pointer, uintptr, *unsafe.Pointer, *uintptr) unsafe.Pointer
	loginStart           func(unsafe.Pointer, uintptr, unsafe.Pointer, uintptr, unsafe.Pointer, uintptr, unsafe.Pointer, uintptr, *unsafe.Pointer, *uintptr, *unsafe.Pointer, *uintptr) unsafe.Pointer
	loginFinish          func(unsafe.Pointer, uintptr, unsafe.Pointer, uintptr) unsafe.Pointer
	getLimits            func(*uintptr, *uintptr, *uintptr, *uintptr, *uintptr) uint8
	freeBytes            func(unsafe.Pointer, uintptr)
	freeError            func(unsafe.Pointer)
}

type ffiLimits struct {
	setup      uintptr
	identifier uintptr
	message    uintptr
	loginState uintptr
	error      uintptr
}

var (
	loadOnce   sync.Once
	loadErr    error
	rustFFI    bindings
	rustLimits ffiLimits
)

func ensureLoaded() error {
	loadOnce.Do(func() {
		path, err := libraryPath()
		if err != nil {
			loadErr = err
			return
		}
		handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
		if err != nil {
			loadErr = fmt.Errorf("load OPAQUE library %q: %w", path, err)
			return
		}
		if err := registerSymbols(handle); err != nil {
			loadErr = err
		}
	})
	return loadErr
}

func registerSymbols(handle uintptr) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("register OPAQUE library symbols: %v", value)
		}
	}()
	purego.RegisterLibFunc(&rustFFI.generateSetup, handle, "opaque_generate_setup")
	purego.RegisterLibFunc(&rustFFI.validateSetup, handle, "opaque_validate_setup")
	purego.RegisterLibFunc(&rustFFI.registrationResponse, handle, "opaque_registration_response")
	purego.RegisterLibFunc(&rustFFI.finishRegistration, handle, "opaque_finish_registration")
	purego.RegisterLibFunc(&rustFFI.loginStart, handle, "opaque_login_start")
	purego.RegisterLibFunc(&rustFFI.loginFinish, handle, "opaque_login_finish")
	purego.RegisterLibFunc(&rustFFI.getLimits, handle, "opaque_get_limits")
	purego.RegisterLibFunc(&rustFFI.freeBytes, handle, "opaque_bytes_free")
	purego.RegisterLibFunc(&rustFFI.freeError, handle, "opaque_free")
	if rustFFI.getLimits(&rustLimits.setup, &rustLimits.identifier, &rustLimits.message, &rustLimits.loginState, &rustLimits.error) != 1 {
		return fmt.Errorf("read OPAQUE FFI limits")
	}
	return nil
}

func libraryPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locate OPAQUE library beside executable: %w", err)
	}
	return filepath.Join(filepath.Dir(executable), libraryName()), nil
}

func libraryName() string {
	switch runtime.GOOS {
	case "darwin":
		return "libarkive_opaque.dylib"
	case "windows":
		return "arkive_opaque.dll"
	default:
		return "libarkive_opaque.so"
	}
}
