use crate::routes::auth::{get_access, register_user};
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
        .mount("/api/v1", routes![get_access, register_user])
        .manage(db_pool)
}
