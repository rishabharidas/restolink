use rocket::{State, http::Status};
use sqlx::PgPool;
use std::io::Result;

use crate::routes::UserInfo;

pub async fn get_user_details(connection_pool: &State<PgPool>, id: &str) -> Result<UserInfo> {
    // get user details here
    println!("retreving user details {:?}", id);
    let customer_id: i32 = id.parse().unwrap();
    let user = sqlx::query_as::<_, UserInfo>("SELECT * FROM restoapp.users WHERE id = $1")
        .bind(customer_id)
        .fetch_one(connection_pool.inner())
        .await
        .map_err(|e| {
            eprintln!("Database error: {}", e);
            Status::InternalServerError
        });
    let user_details = user.unwrap();

    Ok(user_details)
}
