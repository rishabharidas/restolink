use super::{LoginInfo, SignupInfo};
use crate::services::secure::{encrypt_string, get_decrypted_string};
use base64::{Engine, engine::general_purpose::STANDARD};
// use cryptojs_rust::{CryptoOperation, Mode, aes::AesEncryptor};
use rocket::{http::Status, serde::json::Json};
use std::io::Result;

#[post("/login", format = "json", data = "<body>")]
pub fn get_access(body: Json<LoginInfo>) -> Result<Status> {
    let testencryptedpassword =
        "n26AimnrEAdTJWtKFmaVSuAMckE08fE2lWo3hwzUXTPcfctG29nTdigwJIMx+HVHntacnlDwUMGCHf5Iq3Y81A=="; // thing that need to aceesed
    let decrypted_pass = get_decrypted_string(testencryptedpassword).unwrap();
    let login_password_b64 = STANDARD.encode(&body.password);

    if decrypted_pass == login_password_b64 {
        Ok(Status::Ok)
    } else {
        Ok(Status::Unauthorized)
    }
}

#[post("/signup", format = "json", data = "<body>")]
pub fn register_user(body: Json<SignupInfo>) -> Result<Json<bool>> {
    let encrypted = encrypt_string(&body.password).unwrap();
    println!("{:?} <<<<<< saved password", encrypted);

    Ok(Json(true))
}
