use super::{AuthResponse, BasicAuth, LoginInfo, LoginResponse, SignupInfo, UserInfo};
use crate::services::{
    jwt::generate_jwt_token,
    secure::{encrypt_string, get_decrypted_string},
};
use base64::{Engine, engine::general_purpose::STANDARD};
use rocket::{State, http::Status, serde::json::Json};
use sqlx::PgPool;
use std::result::Result;

#[post("/login", format = "json", data = "<body>")]
pub async fn get_access(
    pool: &State<PgPool>,
    body: Json<LoginInfo>,
) -> Result<Json<LoginResponse>, Status> {
    let login_password_b64 = STANDARD.encode(&body.password);

    let user = sqlx::query_as::<_, BasicAuth>(
        "SELECT user_name,password,id FROM restoapp.users WHERE users.user_name = $1",
    )
    .bind(&body.user_name)
    .fetch_one(pool.inner())
    .await
    .expect("Query fetch failed");

    let password = user.password;
    let decrypted_pass = get_decrypted_string(password.as_str()).unwrap();

    if decrypted_pass == login_password_b64 {
        let auth_token = generate_jwt_token(user.id).unwrap();
        Ok(Json({
            LoginResponse {
                status: Status::Ok,
                auth: auth_token,
            }
        }))
    } else {
        Ok(Json({
            LoginResponse {
                status: Status::Unauthorized,
                auth: String::new(),
            }
        }))
    }
}

#[post("/signup", format = "json", data = "<body>")]
pub async fn register_user(
    pool: &State<PgPool>,
    body: Json<SignupInfo>,
) -> Result<Json<AuthResponse>, Status> {
    let encrypted = encrypt_string(&body.password).unwrap();

    // need to add validations,
    // need to add common return reponse
    // need to add seperete function to validate data from db with current

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
