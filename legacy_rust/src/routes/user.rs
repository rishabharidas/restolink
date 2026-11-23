use super::AuthResponse;
use crate::services::{
    jwt::{ApiKey, AuthToken},
    user::get::get_user_details,
};

use rocket::{State, http::Status, serde::json::Json};
use sqlx::PgPool;
use std::result::Result;

#[get("/user/<id>")]
pub async fn get_user_data(
    pool: &State<PgPool>,
    id: &str,
    auth: AuthToken,
    _key: ApiKey,
) -> Result<Json<AuthResponse>, Status> {
    // Retrieve user details from database or other data source
    let user_details = get_user_details(pool, id)
        .await
        .expect("failed to fectch user details");

    if auth.sub != id {
        return Err(Status::Forbidden);
    }

    // Return user details as JSON response
    Ok(Json(AuthResponse {
        status: Status::Ok,
        message: String::from("User Details fetched successfully"),
        data: user_details,
    }))
}
