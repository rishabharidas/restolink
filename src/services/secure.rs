use base64::{Engine, engine::general_purpose::STANDARD};
use dotenvy::dotenv;
use std::{env, io::Result};

use cryptojs_rust::{
    CryptoOperation, Mode,
    aes::{AesDecryptor, AesEncryptor},
};

pub fn get_decrypted_string(key: &str) -> Result<String> {
    dotenv().ok();
    let public_enc_key = env::var("PUBLIC_ENC_KEY").expect("PUBLIC_ENC_KEY must be set");
    let decoded_base64 = STANDARD.decode(key.trim()).unwrap();
    let (salt, rest) = decoded_base64.split_at(16);
    let (iv, ciphertext) = rest.split_at(16);

    let mut decryptor =
        AesDecryptor::new_256_from_password(public_enc_key.as_bytes(), Mode::CBC, salt, Some(iv))
            .unwrap();
    decryptor.update(ciphertext).unwrap();
    Ok(String::from_utf8(decryptor.finalize().unwrap()).unwrap())
}

pub fn encrypt_string(plaintext: &str) -> Result<String> {
    dotenv().ok();
    let public_enc_key = env::var("PUBLIC_ENC_KEY").expect("PUBLIC_ENC_KEY must be set");
    let mut encryptor =
        AesEncryptor::new_256_from_password(public_enc_key.as_bytes(), Mode::CBC).unwrap();
    let og_password_in_64 = STANDARD.encode(&plaintext);
    encryptor.update(og_password_in_64.as_bytes()).unwrap();
    let encrypted = encryptor.finalize().unwrap();
    Ok(STANDARD.encode(encrypted))
}
