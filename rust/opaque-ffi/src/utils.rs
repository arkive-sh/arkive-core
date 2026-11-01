use std::{
    ffi::CString,
    os::raw::c_char,
    panic::{catch_unwind, AssertUnwindSafe},
    ptr, slice, str,
};

use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use zeroize::Zeroize;

// Go reads these through opaque_get_limits so this file is the single source of truth.
pub(super) const MAX_SETUP: usize = 1024;
pub(super) const MAX_IDENTIFIER: usize = 320;
pub(super) const MAX_MESSAGE: usize = 3072;
pub(super) const MAX_LOGIN_STATE: usize = 6144;
pub(super) const MAX_ERROR_LENGTH: usize = 4096;

#[derive(Clone, Copy)]
pub(super) enum ErrorCode {
    InvalidInput,
    InvalidMessage,
    Protocol,
    Internal,
}

impl ErrorCode {
    fn as_str(self) -> &'static str {
        match self {
            Self::InvalidInput => "INVALID_INPUT",
            Self::InvalidMessage => "INVALID_MESSAGE",
            Self::Protocol => "PROTOCOL",
            Self::Internal => "INTERNAL",
        }
    }
}

pub(super) struct FfiError {
    code: ErrorCode,
    message: String,
}

pub(super) type Result<T> = std::result::Result<T, FfiError>;

pub(super) fn ffi_error(code: ErrorCode, message: impl Into<String>) -> FfiError {
    FfiError {
        code,
        message: message.into(),
    }
}

unsafe fn byte_input<'a>(
    value: *const u8,
    length: usize,
    limit: usize,
    allow_empty: bool,
) -> Result<&'a [u8]> {
    if length > limit || (!allow_empty && length == 0) {
        return Err(ffi_error(
            ErrorCode::InvalidInput,
            "input length is invalid",
        ));
    }
    if length == 0 {
        return Ok(&[]);
    }
    if value.is_null() {
        return Err(ffi_error(ErrorCode::InvalidInput, "null input"));
    }
    // Go lends this text buffer for one call; Rust never retains the pointer.
    Ok(unsafe { slice::from_raw_parts(value, length) })
}

pub(super) struct SensitiveInput {
    value: *mut u8,
    length: usize,
}

// Go lends these mutable buffers; dropping the guard clears them after success, errors, or panics.
impl SensitiveInput {
    pub(super) fn as_slice(&self) -> &[u8] {
        if self.length == 0 {
            return &[];
        }
        unsafe { slice::from_raw_parts(self.value, self.length) }
    }
}

impl Drop for SensitiveInput {
    fn drop(&mut self) {
        if self.length > 0 {
            unsafe { slice::from_raw_parts_mut(self.value, self.length) }.zeroize();
        }
    }
}

pub(super) unsafe fn sensitive_input(
    value: *mut u8,
    length: usize,
    limit: usize,
    allow_empty: bool,
) -> Result<SensitiveInput> {
    if length > limit || (!allow_empty && length == 0) {
        return Err(ffi_error(
            ErrorCode::InvalidInput,
            "input length is invalid",
        ));
    }
    if length > 0 && value.is_null() {
        return Err(ffi_error(ErrorCode::InvalidInput, "null input"));
    }
    Ok(SensitiveInput { value, length })
}

pub(super) unsafe fn text_input<'a>(
    value: *const u8,
    length: usize,
    limit: usize,
    allow_empty: bool,
) -> Result<&'a str> {
    str::from_utf8(unsafe { byte_input(value, length, limit, allow_empty)? })
        .map_err(|_| ffi_error(ErrorCode::InvalidInput, "input is not UTF-8"))
}

pub(super) fn decode(value: &str) -> Result<Vec<u8>> {
    URL_SAFE_NO_PAD.decode(value).map_err(|_| {
        ffi_error(
            ErrorCode::InvalidMessage,
            "message is not valid unpadded base64url",
        )
    })
}

fn error_pointer(error: FfiError) -> *mut c_char {
    let message = format!("ERR:{}:{}", error.code.as_str(), error.message);
    let message = if message.len() < MAX_ERROR_LENGTH {
        message
    } else {
        "ERR:INTERNAL:FFI error exceeded its length limit".to_owned()
    };
    CString::new(message)
        .unwrap_or_else(|_| CString::new("ERR:INTERNAL:invalid error output").unwrap())
        .into_raw()
}

fn internal_error(message: &str) -> *mut c_char {
    error_pointer(ffi_error(ErrorCode::Internal, message))
}

pub(super) fn ffi_status(operation: impl FnOnce() -> Result<()>) -> *mut c_char {
    match catch_unwind(AssertUnwindSafe(operation)) {
        Ok(Ok(())) => ptr::null_mut(),
        Ok(Err(error)) => error_pointer(error),
        Err(_) => internal_error("OPAQUE operation panicked"),
    }
}

fn owned_bytes(bytes: Box<[u8]>) -> (*mut u8, usize) {
    if bytes.is_empty() {
        return (ptr::null_mut(), 0);
    }
    let length = bytes.len();
    (Box::into_raw(bytes).cast(), length)
}

pub(super) fn ffi_bytes(
    operation: impl FnOnce() -> Result<Vec<u8>>,
    output: *mut *mut u8,
    output_length: *mut usize,
    output_limit: usize,
) -> *mut c_char {
    if output.is_null() || output_length.is_null() {
        return error_pointer(ffi_error(ErrorCode::InvalidInput, "null output pointer"));
    }
    unsafe {
        *output = ptr::null_mut();
        *output_length = 0;
    }
    match catch_unwind(AssertUnwindSafe(|| {
        let bytes = operation()?;
        if bytes.len() > output_limit {
            return Err(ffi_error(ErrorCode::Internal, "output exceeded its limit"));
        }
        Ok(bytes.into_boxed_slice())
    })) {
        Ok(Ok(bytes)) => {
            let (value, length) = owned_bytes(bytes);
            unsafe {
                *output = value;
                *output_length = length;
            }
            ptr::null_mut()
        }
        Ok(Err(error)) => error_pointer(error),
        Err(_) => internal_error("OPAQUE operation panicked"),
    }
}

pub(super) fn ffi_login_start_bytes(
    operation: impl FnOnce() -> Result<(Vec<u8>, Vec<u8>)>,
    response_output: *mut *mut u8,
    response_length: *mut usize,
    state_output: *mut *mut u8,
    state_length: *mut usize,
) -> *mut c_char {
    if response_output.is_null()
        || response_length.is_null()
        || state_output.is_null()
        || state_length.is_null()
    {
        return error_pointer(ffi_error(ErrorCode::InvalidInput, "null output pointer"));
    }
    unsafe {
        *response_output = ptr::null_mut();
        *response_length = 0;
        *state_output = ptr::null_mut();
        *state_length = 0;
    }
    match catch_unwind(AssertUnwindSafe(|| {
        let (response, state) = operation()?;
        if response.len() > MAX_MESSAGE || state.len() > MAX_LOGIN_STATE {
            return Err(ffi_error(ErrorCode::Internal, "output exceeded its limit"));
        }
        Ok((response.into_boxed_slice(), state.into_boxed_slice()))
    })) {
        Ok(Ok((response, state))) => {
            let (response, response_size) = owned_bytes(response);
            let (state, state_size) = owned_bytes(state);
            unsafe {
                *response_output = response;
                *response_length = response_size;
                *state_output = state;
                *state_length = state_size;
            }
            ptr::null_mut()
        }
        Ok(Err(error)) => error_pointer(error),
        Err(_) => internal_error("OPAQUE operation panicked"),
    }
}
