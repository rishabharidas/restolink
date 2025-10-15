use crate::routes::{
    auth::{get_access, register_user},
    user::get_user_data,
};
use crate::services::database::db_connect;

#[macro_use]
extern crate rocket;

mod routes;
mod services;

#[launch]
async fn rocket() -> _ {
    dotenvy::dotenv().ok();
    let db_pool = db_connect().await.expect("pool connection failed");
    rocket::build()
        .mount("/api/v1", routes![get_access, register_user, get_user_data])
        .manage(db_pool)
}
