use std::{ffi::CString, os::raw::c_char, ptr, slice};

use base64::{engine::general_purpose::URL_SAFE_NO_PAD, Engine};
use opaque_ke::{
    ciphersuite::CipherSuite, CredentialFinalization, CredentialRequest, RegistrationRequest,
    RegistrationUpload, ServerLogin, ServerLoginParameters, ServerRegistration, ServerSetup,
};
use rand::rngs::OsRng;
use zeroize::{Zeroize, Zeroizing};

mod utils;
use utils::{
    decode, ffi_bytes, ffi_error, ffi_login_start_bytes, ffi_status, sensitive_input, text_input,
    ErrorCode, MAX_ERROR_LENGTH, MAX_IDENTIFIER, MAX_LOGIN_STATE, MAX_MESSAGE, MAX_SETUP,
};

struct ArkiveCipherSuite;

impl CipherSuite for ArkiveCipherSuite {
    type OprfCs = opaque_ke::Ristretto255;
    type KeyExchange = opaque_ke::TripleDh<opaque_ke::Ristretto255, sha2::Sha512>;
    type Ksf = argon2::Argon2<'static>;
}

// The exact pointer and length returned by this FFI must be released here, not by Go's allocator.
/// # Safety
/// `value` and `length` must be the exact pair returned by this FFI, and it must not be freed twice.
#[no_mangle]
pub unsafe extern "C" fn opaque_bytes_free(value: *mut u8, length: usize) {
    if !value.is_null() {
        unsafe { slice::from_raw_parts_mut(value, length) }.zeroize();
        let bytes = ptr::slice_from_raw_parts_mut(value, length);
        unsafe { drop(Box::from_raw(bytes)) };
    }
}

/// # Safety
/// Each non-null output pointer must point to writable memory for one `usize`.
#[no_mangle]
pub unsafe extern "C" fn opaque_get_limits(
    setup: *mut usize,
    identifier: *mut usize,
    message: *mut usize,
    login_state: *mut usize,
    error: *mut usize,
) -> u8 {
    if setup.is_null()
        || identifier.is_null()
        || message.is_null()
        || login_state.is_null()
        || error.is_null()
    {
        return 0;
    }
    unsafe {
        setup.write(MAX_SETUP);
        identifier.write(MAX_IDENTIFIER);
        message.write(MAX_MESSAGE);
        login_state.write(MAX_LOGIN_STATE);
        error.write(MAX_ERROR_LENGTH);
    }
    1
}

// Every returned error pointer is allocated by Rust and must be released with this function.
/// # Safety
/// `value` must be a non-null error pointer returned by this FFI, or null.
#[no_mangle]
pub unsafe extern "C" fn opaque_free(value: *mut c_char) {
    if !value.is_null() {
        let mut bytes = unsafe { CString::from_raw(value) }.into_bytes_with_nul();
        bytes.zeroize();
    }
}

/// # Safety
/// `output` and `output_length` must point to writable output locations.
#[no_mangle]
pub unsafe extern "C" fn opaque_generate_setup(
    output: *mut *mut u8,
    output_length: *mut usize,
) -> *mut c_char {
    ffi_bytes(
        || {
            let setup = ServerSetup::<ArkiveCipherSuite>::new(&mut OsRng);
            let serialized = Zeroizing::new(setup.serialize());
            Ok(URL_SAFE_NO_PAD.encode(serialized.as_slice()).into_bytes())
        },
        output,
        output_length,
        MAX_SETUP,
    )
}

/// # Safety
/// `setup` must point to a readable buffer of `setup_length` bytes.
#[no_mangle]
pub unsafe extern "C" fn opaque_validate_setup(
    setup: *const u8,
    setup_length: usize,
) -> *mut c_char {
    ffi_status(|| {
        let setup = unsafe { text_input(setup, setup_length, MAX_SETUP, false)? };
        let setup_bytes = Zeroizing::new(decode(setup)?);
        ServerSetup::<ArkiveCipherSuite>::deserialize(&setup_bytes).map_err(|error| {
            ffi_error(
                ErrorCode::InvalidInput,
                format!("invalid server setup: {error:?}"),
            )
        })?;
        Ok(())
    })
}

/// # Safety
/// Input pointers must reference valid buffers of their supplied lengths; `request` must be writable.
/// Output pointers must point to writable output locations.
#[no_mangle]
pub unsafe extern "C" fn opaque_registration_response(
    setup: *const u8,
    setup_length: usize,
    identifier: *const u8,
    identifier_length: usize,
    request: *mut u8,
    request_length: usize,
    output: *mut *mut u8,
    output_length: *mut usize,
) -> *mut c_char {
    ffi_bytes(
        || {
            let setup = unsafe { text_input(setup, setup_length, MAX_SETUP, false)? };
            let setup_bytes = Zeroizing::new(decode(setup)?);
            let setup =
                ServerSetup::<ArkiveCipherSuite>::deserialize(&setup_bytes).map_err(|error| {
                    ffi_error(
                        ErrorCode::InvalidInput,
                        format!("invalid server setup: {error:?}"),
                    )
                })?;
            let identifier =
                unsafe { text_input(identifier, identifier_length, MAX_IDENTIFIER, false)? };
            let request_bytes =
                unsafe { sensitive_input(request, request_length, MAX_MESSAGE, false)? };
            let request =
                RegistrationRequest::<ArkiveCipherSuite>::deserialize(request_bytes.as_slice())
                    .map_err(|error| {
                        ffi_error(
                            ErrorCode::InvalidMessage,
                            format!("invalid registration request: {error:?}"),
                        )
                    })?;
            let response = ServerRegistration::start(&setup, request, identifier.as_bytes())
                .map_err(|error| {
                    ffi_error(
                        ErrorCode::Protocol,
                        format!("registration failed: {error:?}"),
                    )
                })?;
            Ok(response.message.serialize().to_vec())
        },
        output,
        output_length,
        MAX_MESSAGE,
    )
}

/// # Safety
/// `upload` must reference a writable buffer of `upload_length` bytes. Output pointers must be writable.
#[no_mangle]
pub unsafe extern "C" fn opaque_finish_registration(
    upload: *mut u8,
    upload_length: usize,
    output: *mut *mut u8,
    output_length: *mut usize,
) -> *mut c_char {
    ffi_bytes(
        || {
            let upload_bytes =
                unsafe { sensitive_input(upload, upload_length, MAX_MESSAGE, false)? };
            let upload =
                RegistrationUpload::<ArkiveCipherSuite>::deserialize(upload_bytes.as_slice())
                    .map_err(|error| {
                        ffi_error(
                            ErrorCode::InvalidMessage,
                            format!("invalid registration upload: {error:?}"),
                        )
                    })?;
            // Clear opaque-ke's intermediate record after copying it into the Rust-owned FFI output.
            let serialized = Zeroizing::new(ServerRegistration::finish(upload).serialize());
            Ok(serialized.to_vec())
        },
        output,
        output_length,
        MAX_MESSAGE,
    )
}

/// # Safety
/// Input pointers must reference valid buffers of their supplied lengths; `record` and `request` must be writable.
/// Output pointers must point to writable output locations.
#[no_mangle]
pub unsafe extern "C" fn opaque_login_start(
    setup: *const u8,
    setup_length: usize,
    identifier: *const u8,
    identifier_length: usize,
    record: *mut u8,
    record_length: usize,
    request: *mut u8,
    request_length: usize,
    response_output: *mut *mut u8,
    response_length: *mut usize,
    state_output: *mut *mut u8,
    state_length: *mut usize,
) -> *mut c_char {
    ffi_login_start_bytes(
        || {
            let setup = unsafe { text_input(setup, setup_length, MAX_SETUP, false)? };
            let setup_bytes = Zeroizing::new(decode(setup)?);
            let setup =
                ServerSetup::<ArkiveCipherSuite>::deserialize(&setup_bytes).map_err(|error| {
                    ffi_error(
                        ErrorCode::InvalidInput,
                        format!("invalid server setup: {error:?}"),
                    )
                })?;
            let identifier =
                unsafe { text_input(identifier, identifier_length, MAX_IDENTIFIER, false)? };
            let record = unsafe { sensitive_input(record, record_length, MAX_MESSAGE, true)? };
            let record = if record.as_slice().is_empty() {
                None
            } else {
                Some(
                    ServerRegistration::<ArkiveCipherSuite>::deserialize(record.as_slice())
                        .map_err(|error| {
                            ffi_error(
                                ErrorCode::InvalidMessage,
                                format!("invalid registration record: {error:?}"),
                            )
                        })?,
                )
            };
            let request_bytes =
                unsafe { sensitive_input(request, request_length, MAX_MESSAGE, false)? };
            let request =
                CredentialRequest::<ArkiveCipherSuite>::deserialize(request_bytes.as_slice())
                    .map_err(|error| {
                        ffi_error(
                            ErrorCode::InvalidMessage,
                            format!("invalid login request: {error:?}"),
                        )
                    })?;
            let result = ServerLogin::start(
                &mut OsRng,
                &setup,
                record,
                request,
                identifier.as_bytes(),
                ServerLoginParameters::default(),
            )
            .map_err(|error| {
                ffi_error(
                    ErrorCode::Protocol,
                    format!("login start failed: {error:?}"),
                )
            })?;
            // Clear the serialized login state after copying it into the Rust-owned FFI output.
            let state = Zeroizing::new(result.state.serialize());
            Ok((result.message.serialize().to_vec(), state.to_vec()))
        },
        response_output,
        response_length,
        state_output,
        state_length,
    )
}

/// # Safety
/// `state` and `finalization` must reference writable buffers of their supplied lengths.
#[no_mangle]
pub unsafe extern "C" fn opaque_login_finish(
    state: *mut u8,
    state_length: usize,
    finalization: *mut u8,
    finalization_length: usize,
) -> *mut c_char {
    ffi_status(|| {
        let state_bytes = unsafe { sensitive_input(state, state_length, MAX_LOGIN_STATE, false)? };
        let state = ServerLogin::<ArkiveCipherSuite>::deserialize(state_bytes.as_slice()).map_err(
            |error| {
                ffi_error(
                    ErrorCode::InvalidMessage,
                    format!("invalid login state: {error:?}"),
                )
            },
        )?;
        let finalization_bytes =
            unsafe { sensitive_input(finalization, finalization_length, MAX_MESSAGE, false)? };
        let finalization =
            CredentialFinalization::<ArkiveCipherSuite>::deserialize(finalization_bytes.as_slice())
                .map_err(|error| {
                    ffi_error(
                        ErrorCode::InvalidMessage,
                        format!("invalid login finalization: {error:?}"),
                    )
                })?;
        state
            .finish(finalization, ServerLoginParameters::default())
            .map_err(|error| {
                ffi_error(
                    ErrorCode::Protocol,
                    format!("login verification failed: {error:?}"),
                )
            })?;
        Ok(())
    })
}

#[cfg(test)]
mod tests;
