use super::*;
use crate::utils::Result;
use opaque_ke::{
    ClientLogin, ClientLoginFinishParameters, ClientRegistration,
    ClientRegistrationFinishParameters, CredentialResponse, RegistrationResponse,
};
use std::ffi::CStr;
use std::thread;

fn take_error(value: *mut c_char) -> String {
    assert!(!value.is_null(), "FFI returned a null error pointer");
    let output = unsafe { CStr::from_ptr(value) }
        .to_str()
        .expect("FFI error was not UTF-8")
        .to_owned();
    unsafe { opaque_free(value) };
    output
}

fn success(value: *mut c_char) {
    assert!(
        value.is_null(),
        "unexpected FFI error: {}",
        take_error(value)
    );
}

fn failure(value: *mut c_char, code: &str) {
    let output = take_error(value);
    assert!(
        output.starts_with(&format!("ERR:{code}:")),
        "unexpected FFI result: {output}"
    );
}

fn take_bytes(value: *mut u8, length: usize) -> Vec<u8> {
    if value.is_null() {
        assert_eq!(length, 0, "null output had a non-zero length");
        return Vec::new();
    }
    let output = unsafe { slice::from_raw_parts(value, length) }.to_vec();
    unsafe { opaque_bytes_free(value, length) };
    output
}

fn success_bytes(error: *mut c_char, value: *mut u8, length: usize) -> Vec<u8> {
    success(error);
    take_bytes(value, length)
}

fn generate_setup() -> String {
    let mut output = ptr::null_mut();
    let mut length = 0;
    let error = unsafe { opaque_generate_setup(&mut output, &mut length) };
    String::from_utf8(success_bytes(error, output, length)).unwrap()
}

fn registration_response(setup: &str, username: &str, request: &[u8]) -> Vec<u8> {
    let mut request = request.to_vec();
    let mut output = ptr::null_mut();
    let mut length = 0;
    success_bytes(
        unsafe {
            opaque_registration_response(
                setup.as_ptr(),
                setup.len(),
                username.as_ptr(),
                username.len(),
                request.as_mut_ptr(),
                request.len(),
                &mut output,
                &mut length,
            )
        },
        output,
        length,
    )
}

fn finish_registration(upload: &[u8]) -> Vec<u8> {
    let mut upload = upload.to_vec();
    let mut output = ptr::null_mut();
    let mut length = 0;
    success_bytes(
        unsafe {
            opaque_finish_registration(upload.as_mut_ptr(), upload.len(), &mut output, &mut length)
        },
        output,
        length,
    )
}

fn register(setup: &str, username: &str, password: &str) -> Vec<u8> {
    let mut client_rng = OsRng;
    let start =
        ClientRegistration::<ArkiveCipherSuite>::start(&mut client_rng, password.as_bytes())
            .unwrap();
    let request = start.message.serialize();
    let response = registration_response(setup, username, &request);
    let response = RegistrationResponse::<ArkiveCipherSuite>::deserialize(&response).unwrap();
    let upload = start
        .state
        .finish(
            &mut client_rng,
            password.as_bytes(),
            response,
            ClientRegistrationFinishParameters::default(),
        )
        .unwrap()
        .message
        .serialize();
    finish_registration(&upload)
}

fn login(setup: &str, username: &str, record: &[u8], password: &str) -> bool {
    let mut client_rng = OsRng;
    let start =
        ClientLogin::<ArkiveCipherSuite>::start(&mut client_rng, password.as_bytes()).unwrap();
    let request = start.message.serialize();
    let mut record = record.to_vec();
    let mut request = request;
    let mut response = ptr::null_mut();
    let mut response_length = 0;
    let mut state = ptr::null_mut();
    let mut state_length = 0;
    let error = unsafe {
        opaque_login_start(
            setup.as_ptr(),
            setup.len(),
            username.as_ptr(),
            username.len(),
            record.as_mut_ptr(),
            record.len(),
            request.as_mut_ptr(),
            request.len(),
            &mut response,
            &mut response_length,
            &mut state,
            &mut state_length,
        )
    };
    let response = success_bytes(error, response, response_length);
    let mut state = take_bytes(state, state_length);
    let response = match CredentialResponse::<ArkiveCipherSuite>::deserialize(&response) {
        Ok(value) => value,
        Err(_) => return false,
    };
    let finish = match start.state.finish(
        &mut client_rng,
        password.as_bytes(),
        response,
        ClientLoginFinishParameters::default(),
    ) {
        Ok(value) => value,
        Err(_) => return false,
    };
    let mut finalization = finish.message.serialize();
    let result = unsafe {
        opaque_login_finish(
            state.as_mut_ptr(),
            state.len(),
            finalization.as_mut_ptr(),
            finalization.len(),
        )
    };
    if result.is_null() {
        true
    } else {
        unsafe { opaque_free(result) };
        false
    }
}

#[test]
fn ffi_rejects_bad_and_oversized_inputs_and_catches_panics() {
    failure(
        unsafe { opaque_validate_setup(ptr::null(), 0) },
        "INVALID_INPUT",
    );

    let oversized = "x".repeat(MAX_SETUP + 1);
    failure(
        unsafe { opaque_validate_setup(oversized.as_ptr(), oversized.len()) },
        "INVALID_INPUT",
    );

    let setup = generate_setup();
    let identifier = "test-user";
    let mut oversized = vec![0; MAX_MESSAGE + 1];
    let mut output = ptr::null_mut();
    let mut output_length = 0;
    failure(
        unsafe {
            opaque_registration_response(
                setup.as_ptr(),
                setup.len(),
                identifier.as_ptr(),
                identifier.len(),
                oversized.as_mut_ptr(),
                oversized.len(),
                &mut output,
                &mut output_length,
            )
        },
        "INVALID_INPUT",
    );
    assert!(output.is_null());
    assert_eq!(output_length, 0);

    let mut malformed_upload = [0, 1, 2];
    failure(
        unsafe {
            opaque_finish_registration(
                malformed_upload.as_mut_ptr(),
                malformed_upload.len(),
                &mut output,
                &mut output_length,
            )
        },
        "INVALID_MESSAGE",
    );
    assert_eq!(malformed_upload, [0; 3]);
    failure(
        unsafe { opaque_finish_registration(ptr::null_mut(), 0, &mut output, &mut output_length) },
        "INVALID_INPUT",
    );

    failure(
        ffi_status(|| -> Result<()> { panic!("panic boundary check") }),
        "INTERNAL",
    );
}

#[test]
fn registration_login_password_change_and_parallel_logins_work() {
    let setup = generate_setup();
    success(unsafe { opaque_validate_setup(setup.as_ptr(), setup.len()) });

    let old_record = register(&setup, "test-user", "OldPass-123");
    assert!(login(&setup, "test-user", &old_record, "OldPass-123"));
    assert!(!login(&setup, "test-user", &old_record, "WrongPass-123"));
    assert!(!login(&setup, "different-user", &old_record, "OldPass-123"));

    let new_record = register(&setup, "test-user", "NewPass-456");
    assert!(login(&setup, "test-user", &new_record, "NewPass-456"));
    assert!(!login(&setup, "test-user", &new_record, "OldPass-123"));

    let setup = std::sync::Arc::new(setup);
    let record = std::sync::Arc::new(new_record);
    let threads: Vec<_> = (0..3)
        .map(|_| {
            let setup = std::sync::Arc::clone(&setup);
            let record = std::sync::Arc::clone(&record);
            thread::spawn(move || {
                assert!(login(&setup, "test-user", &record, "NewPass-456"));
            })
        })
        .collect();
    for handle in threads {
        handle.join().expect("concurrent OPAQUE login panicked");
    }
}

#[test]
fn server_rejects_corrupted_login_proof() {
    let setup_value = generate_setup();
    let record_value = register(&setup_value, "test-user", "Pass-12345");
    let mut client_rng = OsRng;
    let start = ClientLogin::<ArkiveCipherSuite>::start(&mut client_rng, b"Pass-12345").unwrap();
    let mut request = start.message.serialize();
    let mut record = record_value.clone();
    let mut response = ptr::null_mut();
    let mut response_length = 0;
    let mut state = ptr::null_mut();
    let mut state_length = 0;
    let error = unsafe {
        opaque_login_start(
            setup_value.as_ptr(),
            setup_value.len(),
            b"test-user".as_ptr(),
            b"test-user".len(),
            record.as_mut_ptr(),
            record.len(),
            request.as_mut_ptr(),
            request.len(),
            &mut response,
            &mut response_length,
            &mut state,
            &mut state_length,
        )
    };
    let response = success_bytes(error, response, response_length);
    let mut state = take_bytes(state, state_length);
    let response = CredentialResponse::<ArkiveCipherSuite>::deserialize(&response).unwrap();
    let finish = start
        .state
        .finish(
            &mut client_rng,
            b"Pass-12345",
            response,
            ClientLoginFinishParameters::default(),
        )
        .unwrap();
    let mut proof = finish.message.serialize().to_vec();
    proof[0] ^= 1;
    let result = unsafe {
        opaque_login_finish(
            state.as_mut_ptr(),
            state.len(),
            proof.as_mut_ptr(),
            proof.len(),
        )
    };
    assert!(!result.is_null(), "corrupted proof was accepted");
    let result = take_error(result);
    assert!(
        result.starts_with("ERR:INVALID_MESSAGE:") || result.starts_with("ERR:PROTOCOL:"),
        "unexpected corrupted proof error: {result}"
    );
}
