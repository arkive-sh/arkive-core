// Package ffi contains memory helpers for synchronous native calls.
package ffi

import (
	"unsafe"
)

// BytePointer returns a pointer to value's bytes, or nil when value is empty.
func BytePointer(value []byte) unsafe.Pointer {
	if len(value) == 0 {
		return nil
	}
	return unsafe.Pointer(unsafe.SliceData(value))
}

// StringPointer returns a pointer to value's bytes, or nil when value is empty.
func StringPointer(value string) unsafe.Pointer {
	if len(value) == 0 {
		return nil
	}
	return unsafe.Pointer(unsafe.StringData(value))
}

// ReadCString copies a null-terminated string up to maxLength bytes.
func ReadCString(pointer unsafe.Pointer, maxLength int) (string, bool) {
	if pointer == nil || maxLength <= 0 {
		return "", false
	}

	message := make([]byte, 0, min(maxLength, 128))
	for offset := 0; offset < maxLength; offset++ {
		value := *(*byte)(unsafe.Add(pointer, uintptr(offset)))
		if value == 0 {
			return string(message), true
		}
		message = append(message, value)
	}
	return "", false
}
