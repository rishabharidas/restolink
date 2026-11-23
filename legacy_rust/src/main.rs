use crate::routes::{
    auth::{get_access, register_user, logout},
    user::get_user_data,
};
use crate::services::database::db_connect;
use crate::services::redis_store::create_pool;

#[macro_use]
extern crate rocket;

mod routes;
mod services;

#[launch]
async fn rocket() -> _ {
    dotenvy::dotenv().ok();
    
    // Postgres Pool
    let db_pool = db_connect().await.expect("pool connection failed");
    
    // Redis Pool
    let redis_pool = create_pool();

    rocket::build()
        .mount("/api/v1", routes![get_access, register_user, get_user_data, logout])
        .manage(db_pool)
        .manage(redis_pool)
}
