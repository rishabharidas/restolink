use serde::{Deserialize, Serialize};

pub mod auth;

#[derive(Serialize, Deserialize, Debug)]
pub struct LoginInfo {
    username: String,
    password: String,
}
