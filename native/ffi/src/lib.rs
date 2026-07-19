use std::{ffi::c_char, panic::AssertUnwindSafe, slice};

static BITWARDEN_SDK_VERSION: &[u8] = b"3.0.0 (7fd530e4852639d7391d062760891631ee9c15c1)\0";

#[repr(C)]
pub struct Buffer {
    ptr: *mut u8,
    len: usize,
}

impl Buffer {
    fn from_vec(mut value: Vec<u8>) -> Self {
        let result = Self {
            ptr: value.as_mut_ptr(),
            len: value.len(),
        };
        std::mem::forget(value);
        result
    }
}

unsafe fn bytes<'a>(ptr: *const u8, len: usize) -> &'a [u8] {
    if len == 0 {
        &[]
    } else {
        unsafe { slice::from_raw_parts(ptr, len) }
    }
}

unsafe fn set_error(output: *mut Buffer, error: impl std::fmt::Display) -> i32 {
    if !output.is_null() {
        unsafe { *output = Buffer::from_vec(format!("{error:#}").into_bytes()) };
    }
    1
}

#[unsafe(no_mangle)]
/// # Safety
/// `buffer` must have been returned by this library and not freed previously.
pub unsafe extern "C" fn bwkp_buffer_free(buffer: Buffer) {
    if !buffer.ptr.is_null() {
        unsafe { drop(Vec::from_raw_parts(buffer.ptr, buffer.len, buffer.len)) };
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn bwkp_bitwarden_sdk_version() -> *const c_char {
    BITWARDEN_SDK_VERSION.as_ptr().cast()
}

#[unsafe(no_mangle)]
/// # Safety
/// Input pointers must reference their declared lengths; output pointers must be writable or null.
pub unsafe extern "C" fn bwkp_login(
    request_ptr: *const u8,
    request_len: usize,
    output: *mut Buffer,
    error: *mut Buffer,
) -> usize {
    let result = std::panic::catch_unwind(AssertUnwindSafe(|| {
        bwkp_bw::login(unsafe { bytes(request_ptr, request_len) })
    }));
    match result {
        Ok(Ok(bwkp_bw::LoginOutcome::Authenticated(session))) => Box::into_raw(session) as usize,
        Ok(Ok(bwkp_bw::LoginOutcome::TwoFactor(providers))) => {
            if !output.is_null() {
                let value = serde_json::to_vec(
                    &serde_json::json!({"type": "two-factor", "providers": providers}),
                )
                .unwrap_or_default();
                unsafe { *output = Buffer::from_vec(value) };
            }
            0
        }
        Ok(Ok(bwkp_bw::LoginOutcome::DeviceVerification(message))) => {
            if !output.is_null() {
                let value = serde_json::to_vec(
                    &serde_json::json!({"type": "device-verification", "message": message}),
                )
                .unwrap_or_default();
                unsafe { *output = Buffer::from_vec(value) };
            }
            0
        }
        Ok(Err(value)) => {
            unsafe { set_error(error, value) };
            0
        }
        Err(_) => {
            unsafe { set_error(error, "native Bitwarden login panicked") };
            0
        }
    }
}

#[unsafe(no_mangle)]
/// # Safety
/// `handle` must be a live session returned by `bwkp_login`; output pointers must be writable or null.
pub unsafe extern "C" fn bwkp_sync(handle: usize, output: *mut Buffer, error: *mut Buffer) -> i32 {
    let Some(session) = (unsafe { (handle as *const bwkp_bw::Session).as_ref() }) else {
        return unsafe { set_error(error, "invalid session") };
    };
    match std::panic::catch_unwind(AssertUnwindSafe(|| bwkp_bw::sync(session))) {
        Ok(Ok(value)) => {
            if !output.is_null() {
                unsafe { *output = Buffer::from_vec(value) };
            }
            0
        }
        Ok(Err(value)) => unsafe { set_error(error, value) },
        Err(_) => unsafe { set_error(error, "native Bitwarden sync panicked") },
    }
}

#[unsafe(no_mangle)]
/// # Safety
/// `handle` must be live, input must reference its declared length, and outputs must be writable or null.
pub unsafe extern "C" fn bwkp_download_attachment(
    handle: usize,
    request_ptr: *const u8,
    request_len: usize,
    output: *mut Buffer,
    error: *mut Buffer,
) -> i32 {
    let Some(session) = (unsafe { (handle as *const bwkp_bw::Session).as_ref() }) else {
        return unsafe { set_error(error, "invalid session") };
    };
    match std::panic::catch_unwind(AssertUnwindSafe(|| {
        bwkp_bw::download_attachment(session, unsafe { bytes(request_ptr, request_len) })
    })) {
        Ok(Ok(value)) => {
            if !output.is_null() {
                unsafe { *output = Buffer::from_vec(value) };
            }
            0
        }
        Ok(Err(value)) => unsafe { set_error(error, value) },
        Err(_) => unsafe { set_error(error, "native attachment download panicked") },
    }
}

#[unsafe(no_mangle)]
/// # Safety
/// `handle` must be live, input must reference its declared length, and outputs must be writable or null.
pub unsafe extern "C" fn bwkp_mutate(
    handle: usize,
    request_ptr: *const u8,
    request_len: usize,
    output: *mut Buffer,
    error: *mut Buffer,
) -> i32 {
    let Some(session) = (unsafe { (handle as *const bwkp_bw::Session).as_ref() }) else {
        return unsafe { set_error(error, "invalid session") };
    };
    match std::panic::catch_unwind(AssertUnwindSafe(|| {
        bwkp_bw::mutate(session, unsafe { bytes(request_ptr, request_len) })
    })) {
        Ok(Ok(value)) => {
            if !output.is_null() {
                unsafe { *output = Buffer::from_vec(value) };
            }
            0
        }
        Ok(Err(value)) => unsafe { set_error(error, value) },
        Err(_) => unsafe { set_error(error, "native Bitwarden mutation panicked") },
    }
}

#[unsafe(no_mangle)]
/// # Safety
/// `handle` must be live, inputs must reference their declared lengths, and outputs must be writable or null.
pub unsafe extern "C" fn bwkp_upload_attachment(
    handle: usize,
    request_ptr: *const u8,
    request_len: usize,
    content_ptr: *const u8,
    content_len: usize,
    output: *mut Buffer,
    error: *mut Buffer,
) -> i32 {
    let Some(session) = (unsafe { (handle as *const bwkp_bw::Session).as_ref() }) else {
        return unsafe { set_error(error, "invalid session") };
    };
    match std::panic::catch_unwind(AssertUnwindSafe(|| {
        bwkp_bw::upload_attachment(
            session,
            unsafe { bytes(request_ptr, request_len) },
            unsafe { bytes(content_ptr, content_len) },
        )
    })) {
        Ok(Ok(value)) => {
            if !output.is_null() {
                unsafe { *output = Buffer::from_vec(value) };
            }
            0
        }
        Ok(Err(value)) => unsafe { set_error(error, value) },
        Err(_) => unsafe { set_error(error, "native attachment upload panicked") },
    }
}

#[unsafe(no_mangle)]
/// # Safety
/// `handle` must be zero or a live session returned by `bwkp_login` and not previously closed.
pub unsafe extern "C" fn bwkp_session_close(handle: usize) {
    if handle != 0 {
        unsafe { drop(Box::from_raw(handle as *mut bwkp_bw::Session)) };
    }
}
