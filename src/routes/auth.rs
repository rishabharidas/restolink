use super::{AuthResponse, BasicAuth, LoginInfo, LoginResponse, SignupInfo, UserInfo};
use crate::services::{
    jwt::{ApiKey, AuthToken, generate_jwt_token},
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
    _key: ApiKey,
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
                message: "Login Sucessfull".to_string(),
                auth: auth_token,
            }
        }))
    } else {
        Ok(Json({
            LoginResponse {
                status: Status::Unauthorized,
                message: "Login Failed".to_string(),
                auth: String::new(),
            }
        }))
    }
}

#[post("/signup", format = "json", data = "<body>")]
pub async fn register_user(
    pool: &State<PgPool>,
    body: Json<SignupInfo>,
    _key: ApiKey,
) -> Result<Json<AuthResponse>, Status> {
    // encrypt password, return 500 if encryption fails
    let encrypted = encrypt_string(&body.password).map_err(|e| {
        eprintln!("Encryption error: {}", e);
        Status::InternalServerError
    })?;

    // NOTE: Using the pool as the executor for queries (pool.inner()) to satisfy sqlx Executor bounds.
    // This removes the explicit connection/transaction handling that previously caused trait issues.
    // Atomicity: without an explicit transaction, these operations are not strictly atomic. If you need
    // transactional guarantees later, we can reintroduce transactions using the appropriate types/patterns.

    // 1) Try to find existing role id by name
    let role_id_opt: Option<i32> =
        sqlx::query_scalar("SELECT id FROM restoapp.roles WHERE name = $1")
            .bind(&body.role)
            .fetch_optional(pool.inner())
            .await
            .map_err(|e| {
                eprintln!("Database error while looking up role: {}", e);
                Status::InternalServerError
            })?;

    // 2) If not found, create the role and get its id //TODOc  update throw error only allow roles that are present
    let role_id: i32 = match role_id_opt {
        Some(id) => id,
        None => {
            let inserted_role_id: i32 =
                sqlx::query_scalar("INSERT INTO restoapp.roles (name) VALUES ($1) RETURNING id")
                    .bind(&body.role)
                    .fetch_one(pool.inner())
                    .await
                    .map_err(|e| {
                        eprintln!("Database error while inserting role: {}", e);
                        Status::InternalServerError
                    })?;
            inserted_role_id
        }
    };

    // 3) Insert the user with resolved role_id and get the new user's id
    let user_id: i32 = sqlx::query_scalar(
        "INSERT INTO restoapp.users (email, user_name, first_name, last_name, password, \"number\", role_id) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id",
    )
    .bind(&body.email)
    .bind(&body.user_name)
    .bind(&body.first_name)
    .bind(&body.last_name)
    .bind(&encrypted)
    .bind(&body.number)
    .bind(role_id)
    .fetch_one(pool.inner())
    .await
    .map_err(|e| {
        eprintln!("Database error while inserting user: {}", e);
        Status::InternalServerError
    })?;

    // 4) Fetch the inserted user along with the role name to populate UserInfo (which expects role as String)
    let inserted_user = sqlx::query_as::<_, UserInfo>(
        "SELECT u.user_name, u.first_name, u.last_name, u.email, u.number, u.id, r.name as role
         FROM restoapp.users u
         JOIN restoapp.roles r ON u.role_id = r.id
         WHERE u.id = $1",
    )
    .bind(user_id)
    .fetch_one(pool.inner())
    .await
    .map_err(|e| {
        eprintln!("Database error while fetching inserted user: {}", e);
        Status::InternalServerError
    })?;

    Ok(Json(AuthResponse {
        status: Status::Ok,
        message: "User registered successfully".to_string(),
        data: inserted_user,
    }))
}

#[derive(serde::Serialize)]
pub struct LogoutResponse {
    pub status: String,
    pub message: String,
}

#[post("/logout")]
pub async fn logout(
    auth: AuthToken, // Ensures they are logged in to begin with; contains the raw token
    redis_pool: &State<deadpool_redis::Pool>,
) -> Result<Json<LogoutResponse>, Status> {
    // Use the raw token provided by the AuthToken request guard
    let raw_token = auth.raw.as_str();

    // Calculate remaining Time-To-Live (TTL)
    let current_time = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_secs() as i64;

    let ttl_seconds = auth.exp - current_time;

    // Blacklist it
    if let Err(e) =
        crate::services::redis_store::blacklist_token(redis_pool.inner(), raw_token, ttl_seconds)
            .await
    {
        eprintln!("Failed to blacklist token: {}", e);
        return Err(Status::InternalServerError);
    }

    Ok(Json(LogoutResponse {
        status: "success".to_string(),
        message: "Successfully logged out".to_string(),
    }))
}
