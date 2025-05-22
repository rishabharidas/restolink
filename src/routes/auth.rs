use super::{LoginInfo, SignupInfo};
use base64::{Engine, engine::general_purpose::STANDARD};
use cryptojs_rust::{
    CryptoOperation, Mode,
    aes::{AesDecryptor, AesEncryptor},
};
use rocket::{http::Status, serde::json::Json};
use std::io::Result;

#[post("/login", format = "json", data = "<body>")]
pub fn get_access(body: Json<LoginInfo>) -> Result<Status> {
    let transfer_pass = b"this is private transfer password"; // need to update with env
    let testencryptedpassword =
        "n26AimnrEAdTJWtKFmaVSuAMckE08fE2lWo3hwzUXTPcfctG29nTdigwJIMx+HVHntacnlDwUMGCHf5Iq3Y81A=="; // thing that need to aceesed

    let login_password_b64 = STANDARD.encode(&body.password);

    let decrypted_password = STANDARD.decode(testencryptedpassword).unwrap();
    let (salt, rest) = decrypted_password.split_at(16);
    let (iv, ciphertext) = rest.split_at(16);

    let mut decryptor =
        AesDecryptor::new_256_from_password(transfer_pass, Mode::CBC, salt, Some(iv)).unwrap();
    decryptor.update(ciphertext).unwrap();
    let decrypted = decryptor.finalize().unwrap();
    let password = String::from_utf8(decrypted).unwrap();

    if password == login_password_b64 {
        Ok(Status::Ok)
    } else {
        Ok(Status::Unauthorized)
    }
}

#[post("/signup", format = "json", data = "<body>")]
pub fn register_user(body: Json<SignupInfo>) -> Result<Json<bool>> {
    let transfer_pass = b"this is private transfer password"; // need to update with env

    let mut encryptor = AesEncryptor::new_256_from_password(transfer_pass, Mode::CBC).unwrap();
    let og_password_in_64 = STANDARD.encode(&body.password);
    encryptor.update(og_password_in_64.as_bytes()).unwrap();
    let encrypted = encryptor.finalize().unwrap();

    let passwordbase64 = STANDARD.encode(encrypted);
    println!("{:?} <<<<<< saved password", passwordbase64);

    Ok(Json(true))
}
