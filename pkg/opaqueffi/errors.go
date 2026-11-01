// Package opaqueffi decodes values returned by Arkive's OPAQUE FFI.
package opaqueffi

import (
	"strings"
	"unsafe"

	"arkive/pkg/ffi"
)

// ErrorCode identifies a failure reported by the OPAQUE Rust FFI.
type ErrorCode string

// These codes are returned in the Rust FFI error envelope.
const (
	ErrorInvalidInput   ErrorCode = "INVALID_INPUT"
	ErrorInvalidMessage ErrorCode = "INVALID_MESSAGE"
	ErrorProtocol       ErrorCode = "PROTOCOL"
	ErrorInternal       ErrorCode = "INTERNAL"
)

// Error is a decoded OPAQUE FFI failure.
type Error struct {
	Code    ErrorCode
	Message string
}

// Buffer is a Go handle to memory allocated, zeroized, and released by Rust.
type Buffer struct {
	pointer unsafe.Pointer
	length  uintptr
	free    func(unsafe.Pointer, uintptr)
}

func (e *Error) Error() string {
	return "OPAQUE " + string(e.Code) + ": " + e.Message
}

// ReadError copies, frees, and decodes an FFI-owned error string.
func ReadError(pointer unsafe.Pointer, maxLength int, free func(unsafe.Pointer)) error {
	if pointer == nil {
		return nil
	}
	message, terminated := ffi.ReadCString(pointer, maxLength)
	free(pointer)
	if !terminated {
		return &Error{Code: ErrorInternal, Message: "Rust returned an unterminated error"}
	}
	if !strings.HasPrefix(message, "ERR:") {
		return &Error{Code: ErrorInternal, Message: "Rust returned an invalid error"}
	}
	parts := strings.SplitN(strings.TrimPrefix(message, "ERR:"), ":", 2)
	if len(parts) != 2 {
		return &Error{Code: ErrorInternal, Message: "malformed FFI error"}
	}
	return &Error{Code: ErrorCode(parts[0]), Message: parts[1]}
}

// Bytes returns a view of Rust-owned memory that is valid until Close.
func (b *Buffer) Bytes() []byte {
	if b == nil || b.pointer == nil || b.length == 0 {
		return nil
	}
	return unsafe.Slice((*byte)(b.pointer), int(b.length))
}

// Close releases the buffer through its Rust allocator; call it after the final use of Bytes.
func (b *Buffer) Close() {
	if b == nil || b.pointer == nil {
		return
	}
	pointer, length, free := b.pointer, b.length, b.free
	b.pointer, b.length, b.free = nil, 0, nil
	free(pointer, length)
}

// ReadOutput wraps a Rust buffer without copying its contents into Go memory.
func ReadOutput(pointer unsafe.Pointer, length, limit uintptr, free func(unsafe.Pointer, uintptr)) (*Buffer, error) {
	if pointer == nil {
		if length != 0 {
			return nil, &Error{Code: ErrorInternal, Message: "FFI returned an invalid output"}
		}
		return &Buffer{}, nil
	}
	if length == 0 || free == nil {
		return nil, &Error{Code: ErrorInternal, Message: "FFI returned an invalid output"}
	}
	if length > limit {
		free(pointer, length)
		return nil, &Error{Code: ErrorInternal, Message: "FFI returned oversized output"}
	}
	return &Buffer{pointer: pointer, length: length, free: free}, nil
}
