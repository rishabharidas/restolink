use dotenvy::dotenv;
use jsonwebtoken::{Algorithm, DecodingKey, EncodingKey, Header, Validation, decode, encode};
use serde::{Deserialize, Serialize};
use std::time::{SystemTime, UNIX_EPOCH};
use std::{env, io::Result};

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct Claims {
    sub: String,
    exp: i64,
}

pub fn generate_jwt_token(user_id: i32) -> Result<String> {
    dotenv().ok();

    let jwt_secret_key = env::var("JWT_SECRET").expect("JWT_SECRET must be set");
    let current_time = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .expect("tim eis after")
        .as_secs() as i64;
    let header = Header::new(Algorithm::HS256);
    let claims = Claims {
        sub: user_id.to_string(),
        exp: current_time + 3600 * 24 * 7,
    };
    let token = encode(
        &header,
        &claims,
        &EncodingKey::from_secret(jwt_secret_key.as_bytes()),
    )
    .expect("token generation failed");
    Ok(token)
}

pub fn decode_jwt_token(token: &str) -> Result<Claims> {
    dotenv().ok();

    let jwt_secret_key = env::var("JWT_SECRET").expect("JWT_SECRET must be set");
    let token_data = decode::<Claims>(
        token,
        &DecodingKey::from_secret(jwt_secret_key.as_bytes()),
        &Validation::new(Algorithm::HS256),
    )
    .expect("token decoding failed");
    Ok(token_data.claims)
}
