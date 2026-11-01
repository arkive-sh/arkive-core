package opaque

import (
	"runtime"
	"unsafe"

	"arkive/pkg/ffi"
	"arkive/pkg/opaqueffi"
)

type ErrorCode = opaqueffi.ErrorCode
type Error = opaqueffi.Error

const (
	ErrorInvalidInput   = opaqueffi.ErrorInvalidInput
	ErrorInvalidMessage = opaqueffi.ErrorInvalidMessage
	ErrorProtocol       = opaqueffi.ErrorProtocol
	ErrorInternal       = opaqueffi.ErrorInternal
)

type LoginStart struct {
	Response *opaqueffi.Buffer
	State    *opaqueffi.Buffer
}

// GenerateSetup returns the server setup in Rust-owned memory; the caller must close it.
func GenerateSetup() (*opaqueffi.Buffer, error) {
	if err := ensureLoaded(); err != nil {
		return nil, err
	}
	var output unsafe.Pointer
	var outputLength uintptr
	if err := opaqueffi.ReadError(rustFFI.generateSetup(&output, &outputLength), int(rustLimits.error), rustFFI.freeError); err != nil {
		return nil, err
	}
	return opaqueffi.ReadOutput(output, outputLength, rustLimits.setup, rustFFI.freeBytes)
}

func ValidateSetup(setup string) error {
	if err := ensureLoaded(); err != nil {
		return err
	}
	if err := validateLength(len(setup), rustLimits.setup, false); err != nil {
		return err
	}
	result := rustFFI.validateSetup(ffi.StringPointer(setup), uintptr(len(setup)))
	runtime.KeepAlive(setup)
	return opaqueffi.ReadError(result, int(rustLimits.error), rustFFI.freeError)
}

// RegistrationResponse consumes request; Rust zeroizes its backing bytes before returning.
func RegistrationResponse(setup, identifier string, request []byte) (*opaqueffi.Buffer, error) {
	if err := ensureLoaded(); err != nil {
		return nil, err
	}
	if err := validateLength(len(setup), rustLimits.setup, false); err != nil {
		return nil, err
	}
	if err := validateLength(len(identifier), rustLimits.identifier, false); err != nil {
		return nil, err
	}
	if err := validateLength(len(request), rustLimits.message, false); err != nil {
		return nil, err
	}
	var output unsafe.Pointer
	var outputLength uintptr
	result := rustFFI.registrationResponse(
		ffi.StringPointer(setup),
		uintptr(len(setup)),
		ffi.StringPointer(identifier),
		uintptr(len(identifier)),
		ffi.BytePointer(request),
		uintptr(len(request)),
		&output,
		&outputLength,
	)
	runtime.KeepAlive(setup)
	runtime.KeepAlive(identifier)
	runtime.KeepAlive(request)
	if err := opaqueffi.ReadError(result, int(rustLimits.error), rustFFI.freeError); err != nil {
		return nil, err
	}
	return opaqueffi.ReadOutput(output, outputLength, rustLimits.message, rustFFI.freeBytes)
}

// FinishRegistration consumes upload; Rust zeroizes its backing bytes before returning.
func FinishRegistration(upload []byte) (*opaqueffi.Buffer, error) {
	if err := ensureLoaded(); err != nil {
		return nil, err
	}
	if err := validateLength(len(upload), rustLimits.message, false); err != nil {
		return nil, err
	}
	var output unsafe.Pointer
	var outputLength uintptr
	result := rustFFI.finishRegistration(ffi.BytePointer(upload), uintptr(len(upload)), &output, &outputLength)
	runtime.KeepAlive(upload)
	if err := opaqueffi.ReadError(result, int(rustLimits.error), rustFFI.freeError); err != nil {
		return nil, err
	}
	return opaqueffi.ReadOutput(output, outputLength, rustLimits.message, rustFFI.freeBytes)
}

// StartLogin consumes and zeroizes record and request before returning.
func StartLogin(setup, identifier string, record, request []byte) (LoginStart, error) {
	if err := ensureLoaded(); err != nil {
		return LoginStart{}, err
	}
	if err := validateLength(len(setup), rustLimits.setup, false); err != nil {
		return LoginStart{}, err
	}
	if err := validateLength(len(identifier), rustLimits.identifier, false); err != nil {
		return LoginStart{}, err
	}
	if err := validateLength(len(record), rustLimits.message, true); err != nil {
		return LoginStart{}, err
	}
	if err := validateLength(len(request), rustLimits.message, false); err != nil {
		return LoginStart{}, err
	}
	var responseOutput, stateOutput unsafe.Pointer
	var responseLength, stateLength uintptr
	result := rustFFI.loginStart(
		ffi.StringPointer(setup),
		uintptr(len(setup)),
		ffi.StringPointer(identifier),
		uintptr(len(identifier)),
		ffi.BytePointer(record),
		uintptr(len(record)),
		ffi.BytePointer(request),
		uintptr(len(request)),
		&responseOutput,
		&responseLength,
		&stateOutput,
		&stateLength,
	)
	runtime.KeepAlive(setup)
	runtime.KeepAlive(identifier)
	runtime.KeepAlive(record)
	runtime.KeepAlive(request)
	if err := opaqueffi.ReadError(result, int(rustLimits.error), rustFFI.freeError); err != nil {
		return LoginStart{}, err
	}
	response, err := opaqueffi.ReadOutput(responseOutput, responseLength, rustLimits.message, rustFFI.freeBytes)
	if err != nil {
		return LoginStart{}, err
	}
	state, err := opaqueffi.ReadOutput(stateOutput, stateLength, rustLimits.loginState, rustFFI.freeBytes)
	if err != nil {
		response.Close()
		return LoginStart{}, err
	}
	return LoginStart{Response: response, State: state}, nil
}

// FinishLogin consumes and zeroizes state and finalization before returning.
func FinishLogin(state, finalization []byte) error {
	if err := ensureLoaded(); err != nil {
		return err
	}
	if err := validateLength(len(state), rustLimits.loginState, false); err != nil {
		return err
	}
	if err := validateLength(len(finalization), rustLimits.message, false); err != nil {
		return err
	}
	result := rustFFI.loginFinish(ffi.BytePointer(state), uintptr(len(state)), ffi.BytePointer(finalization), uintptr(len(finalization)))
	runtime.KeepAlive(state)
	runtime.KeepAlive(finalization)
	return opaqueffi.ReadError(result, int(rustLimits.error), rustFFI.freeError)
}

func validateLength(length int, limit uintptr, allowEmpty bool) error {
	if uintptr(length) > limit || (!allowEmpty && length == 0) {
		return &Error{Code: ErrorInvalidInput, Message: "input length is invalid"}
	}
	return nil
}
