use crate::routes::auth::get_access;

#[macro_use]
extern crate rocket;

mod routes;

#[launch]
fn rocket() -> _ {
    rocket::build().mount("/api/v1", routes![get_access])
}
