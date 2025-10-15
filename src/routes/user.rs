use crate::services::user::get::get_user_details;

use rocket::{http::Status, serde::json::Json};
use sqlx::PgPool;
use std::result::Result;

#[get("/user/<id>")]
pub fn get_user_data(id: &str) -> Result<Status, Status> {
    // Retrieve user details from database or other data source
    get_user_details(id);

    // Return user details as JSON response
    Ok(Status::Ok)
}
