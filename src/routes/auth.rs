use super::{LoginInfo, SignupInfo};
use crate::services::secure::{encrypt_string, get_decrypted_string};
use base64::{Engine, engine::general_purpose::STANDARD};
use rocket::{State, http::Status, serde::json::Json};
use sqlx::PgPool;
use std::io::Result;

#[post("/login", format = "json", data = "<body>")]
pub async fn get_access(pool: &State<PgPool>, body: Json<LoginInfo>) -> Result<Status> {
    // let testencryptedpassword =
    //     "n26AimnrEAdTJWtKFmaVSuAMckE08fE2lWo3hwzUXTPcfctG29nTdigwJIMx+HVHntacnlDwUMGCHf5Iq3Y81A=="; // thing that need to aceesed
    // let decrypted_pass = get_decrypted_string(testencryptedpassword).unwrap();
    let login_password_b64 = STANDARD.encode(&body.password);

    let user = sqlx::query("SELECT * FROM restoapp.users WHERE user.user_name = $1")
        .bind(&body.username)
        .fetch_all(pool.inner())
        .await
        .expect("Query fetch failed");
    println!("{:?} {:?}", user, body);

    if decrypted_pass == login_password_b64 {
        Ok(Status::Ok)
    } else {
        Ok(Status::Unauthorized)
    }
}

#[post("/signup", format = "json", data = "<body>")]
pub async fn register_user(
    pool: &State<PgPool>,
    body: Json<SignupInfo>,
) -> std::result::Result<Json<bool>, Status> {
    // FIX 1: encrypt_string should be awaited if async, but assuming it's sync for now.
    let encrypted = encrypt_string(&body.password).unwrap();

    // The pool is wrapped in State, so use pool.inner() to access the PgPool
    let test_query_result = sqlx::query("SELECT 'Hello, World!'")
        .execute(pool.inner())
        .await
        .map_err(|e| {
            eprintln!("Database error: {}", e);
            Status::InternalServerError
        })?;

    // FIX 2: Correct println! macro usage. test_query_result is the variable you want to print.
    println!("Test Query Result: {:?}", test_query_result);

    // FIX 3: Variable 'res' was undeclared. It seems you intended to use 'encrypted' or the query result.
    // If you were expecting the result of the query in 'res', use 'test_query_result'.
    println!(
        "{:?} <<<<<< saved password (Query result: {:?})",
        encrypted, test_query_result
    );

    // TODO: Insert user into the database here using the pool

    Ok(Json(true))
}
