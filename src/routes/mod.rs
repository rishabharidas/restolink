use serde::{Deserialize, Serialize};
use sqlx::FromRow;

pub mod auth;

#[derive(Serialize, Deserialize, Debug)]
pub struct LoginInfo {
    user_name: String,
    password: String,
}

#[derive(Debug, FromRow, Serialize)]
pub struct BasicAuth {
    pub user_name: String,
    pub password: String,
}

#[derive(Serialize, Deserialize, Debug)]
pub struct SignupInfo {
    user_name: String,
    password: String,
    role: String,
    first_name: String,
    last_name: String,
    email: String,
    number: i64,
}

#[derive(Debug, FromRow, Serialize)]
pub struct UserInfo {
    pub user_name: String,
    pub first_name: String,
    pub last_name: String,
    pub email: String,
    pub number: i64,
    pub id: i32,
}
