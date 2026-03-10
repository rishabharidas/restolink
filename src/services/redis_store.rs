use deadpool_redis::{Config, Runtime};
use redis::AsyncCommands;
use sha2::{Digest, Sha256};
use std::env;

/// Creates a Redis connection pool.
pub fn create_pool() -> deadpool_redis::Pool {
    let redis_url = env::var("REDIS_URL").unwrap_or_else(|_| "redis://127.0.0.1:6379".to_string());
    let cfg = Config::from_url(redis_url);
    cfg.create_pool(Some(Runtime::Tokio1)).expect("Failed to create Redis pool")
}

/// Hashes a token using SHA-256 to prevent storing raw tokens.
fn hash_token(token: &str) -> String {
    let mut hasher = Sha256::new();
    hasher.update(token.as_bytes());
    let result = hasher.finalize();
    hex::encode(result)
}

/// Adds a token hash to Redis with a TTL (Time-To-Live).
pub async fn blacklist_token(
    pool: &deadpool_redis::Pool,
    token: &str,
    ttl_seconds: i64,
) -> Result<(), deadpool_redis::PoolError> {
    if ttl_seconds <= 0 {
        return Ok(()); // Already expired, no need to blacklist
    }

    let hashed_token = hash_token(token);
    let mut conn = pool.get().await?;

    // Store in Redis using SET with EX (expiration)
    // The value doesn't matter much, "1" is fine.
    let _: () = conn.set_ex(format!("blacklist:{}", hashed_token), "1", ttl_seconds as u64).await.unwrap_or_else(|e| {
        eprintln!("Failed to store token in Redis: {}", e);
    });

    Ok(())
}

/// Checks if a token's hash exists in the Redis blacklist.
pub async fn is_token_blacklisted(
    pool: &deadpool_redis::Pool,
    token: &str,
) -> Result<bool, deadpool_redis::PoolError> {
    let hashed_token = hash_token(token);
    let mut conn = pool.get().await?;

    // Check if key exists
    let exists: bool = conn.exists(format!("blacklist:{}", hashed_token)).await.unwrap_or(false);
    
    Ok(exists)
}
