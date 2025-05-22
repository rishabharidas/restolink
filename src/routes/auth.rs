use super::LoginInfo;
use base64::{Engine, engine::general_purpose::STANDARD};
use cryptojs_rust::{CryptoOperation, Mode, aes::AesEncryptor};
use rocket::serde::json::Json;
use std::io::Result;

#[post("/login", format = "json", data = "<body>")]
pub fn get_access(body: Json<LoginInfo>) -> Result<Json<bool>> {
    let transfer_pass: &[u8] = b"this is private transfer password";
    let mut encryptor = AesEncryptor::new_256_from_password(transfer_pass, Mode::CBC).unwrap();
    encryptor.update(body.password.as_bytes()).unwrap();
    let encrypted = encryptor.finalize().unwrap();

    let bases64 = STANDARD.encode(&encrypted);

    // let salt = &encrypted[..16];
    // let iv = &encrypted[16..32];
    // let ciphertext = &encrypted[32..];

    println!("{:?} login daat", body);
    println!("{:?}", bases64);
    Ok(Json(true))
}
