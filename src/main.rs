use crate::routes::auth::{get_access, register_user};

#[macro_use]
extern crate rocket;

mod routes;
mod services;

#[launch]
fn rocket() -> _ {
    rocket::build().mount("/api/v1", routes![get_access, register_user])
}
