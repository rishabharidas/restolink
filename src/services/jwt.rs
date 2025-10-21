use jsonwebtoken::{Algorithm, DecodingKey, EncodingKey, Header, Validation, decode, encode};
use rocket::http::Status;
use rocket::request::{FromRequest, Outcome, Request};
use serde::{Deserialize, Serialize};
use std::time::{SystemTime, UNIX_EPOCH};
use std::{env, io::Result};

#[derive(Debug, Serialize, Deserialize, Clone)]
pub struct Claims {
    sub: String,
    exp: i64,
}

pub struct ApiKey {
    pub sub: String,
    pub exp: i64,
}

pub fn generate_jwt_token(user_id: i32) -> Result<String> {
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
    let jwt_secret_key = env::var("JWT_SECRET").expect("JWT_SECRET must be set");
    let token_data = decode::<Claims>(
        token,
        &DecodingKey::from_secret(jwt_secret_key.as_bytes()),
        &Validation::new(Algorithm::HS256),
    )
    .expect("token decoding failed");
    Ok(token_data.claims)
}

#[derive(Debug)]
pub enum ApiKeyError {
    Missing,
    Invalid,
}

#[rocket::async_trait]
impl<'r> FromRequest<'r> for ApiKey {
    type Error = ApiKeyError;

    async fn from_request(req: &'r Request<'_>) -> Outcome<Self, Self::Error> {
        let auth_header_value = req.headers().get_one("Authorization");

        // 2. Check if the header exists
        let header_value = match auth_header_value {
            Some(v) => v,
            None => return Outcome::Error((Status::BadRequest, ApiKeyError::Missing)),
        };

        let token = header_value.strip_prefix("Bearer ");

        let token_value = match token {
            Some(v) => v,
            None => return Outcome::Error((Status::BadRequest, ApiKeyError::Missing)),
        };

        let is_valid = validate_token(token_value).await;

        match is_valid {
            Some(Claims { sub, exp }) => {
                // Correctly destructure the ApiKey struct
                // Token is valid! Return the Guard instance.
                Outcome::Success(ApiKey { sub, exp })
            }
            None => {
                // Token is invalid or expired.
                Outcome::Error((Status::BadRequest, ApiKeyError::Missing))
            }
        }
    }
}

async fn validate_token(token: &str) -> Option<Claims> {
    let decoded_token_data = decode_jwt_token(token);
    println!("{:?}", decoded_token_data);
    if token.len() > 10 {
        Some(decoded_token_data.unwrap())
    } else {
        None
    }
}
