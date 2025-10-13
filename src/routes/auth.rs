use super::{BasicAuth, LoginInfo, SignupInfo, UserInfo};
use crate::services::secure::{encrypt_string, get_decrypted_string};
use base64::{Engine, engine::general_purpose::STANDARD};
use rocket::{
    State,
    http::Status,
    serde::{Serialize, json::Json},
};
use sqlx::PgPool;
use std::io::Result;

#[derive(Serialize)]
pub struct AuthResponse {
    status: Status,
    message: String,
    data: UserInfo,
}

#[post("/login", format = "json", data = "<body>")]
pub async fn get_access(pool: &State<PgPool>, body: Json<LoginInfo>) -> Result<Status> {
    let login_password_b64 = STANDARD.encode(&body.password);

    let user = sqlx::query_as::<_, BasicAuth>(
        "SELECT user_name,password FROM restoapp.users WHERE users.user_name = $1",
    )
    .bind(&body.user_name)
    .fetch_one(pool.inner())
    .await
    .expect("Query fetch failed");

    let password = user.password;
    let decrypted_pass = get_decrypted_string(password.as_str()).unwrap();

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
) -> std::result::Result<Json<AuthResponse>, Status> {
    let encrypted = encrypt_string(&body.password).unwrap();

    let insert_user_query = sqlx::query_as::<_, UserInfo>("INSERT INTO restoapp.users (email, user_name, first_name, last_name, password, number, role) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id, user_name, email, first_name, last_name, number")
        .bind(&body.email)
        .bind(&body.user_name)
        .bind(&body.first_name)
        .bind(&body.last_name)
        .bind(&encrypted)
        .bind(&body.number)
        .bind(&body.role)
        .fetch_one(pool.inner())
        .await
        .map_err(|e| {
            eprintln!("Database error: {}", e);
            Status::InternalServerError
        })?;

    Ok(Json({
        AuthResponse {
            status: Status::Ok,
            message: "User registered successfully".to_string(),
            data: insert_user_query,
        }
    }))
}
