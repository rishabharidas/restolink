use sqlx::postgres::PgPool;
use sqlx::{Pool, Postgres};
use std::env;

// Re-exports Result and Error for simpler error handling in this function signature
// You would typically define your own error type for a real application.
pub type Result<T> = std::result::Result<T, sqlx::Error>;

pub async fn db_connect() -> Result<Pool<Postgres>> {
    // 1. Read the database URL from the environment
    //    We use env::var to get the DATABASE_URL.
    //    We'll handle the potential missing variable with a simple panic for this example.
    let database_url = env::var("DATABASE_URL")
        .expect("DATABASE_URL must be set in the environment or .env file");

    println!("Attempting to connect to PostgreSQL using URL from environment...");

    // 2. Establish the connection pool
    //    PgPool::connect() creates a connection pool and attempts an initial connection.
    let pool = PgPool::connect(&database_url).await?;

    println!("Successfully established connection pool.");

    // The pool is the object you pass around and use for database operations.
    // It automatically manages connections and performs connection pooling.
    Ok(pool)
}
