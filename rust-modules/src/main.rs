mod api;
mod features;
mod shared;

use std::env;
use std::io::{Read, Write};
use std::net::{TcpListener, TcpStream};
use std::sync::Arc;
use std::thread;

use api::rest::v1::handlers::health_checks_handler::HealthChecksHandler;
use api::rest::v1::routers::health_checks_router::HealthChecksRouter;
use features::health_checks::{HealthChecksRepositoryAdapter, HealthChecksService};
use shared::constants::{DEFAULT_ADMIN_HOST, DEFAULT_ADMIN_PORT, DEFAULT_HOST, DEFAULT_PORT, SERVICE_NAME};

fn handle_connection(mut stream: TcpStream, router: Arc<HealthChecksRouter>, is_admin: bool) {
    let mut buffer = [0; 2048];
    if let Ok(bytes_read) = stream.read(&mut buffer) {
        if bytes_read == 0 {
            return;
        }

        let request = String::from_utf8_lossy(&buffer[..bytes_read]);
        let mut lines = request.lines();
        let request_line = match lines.next() {
            Some(line) => line,
            None => return,
        };

        let mut parts = request_line.split_whitespace();
        let method = parts.next().unwrap_or("GET");
        let path = parts.next().unwrap_or("/");

        let trace_id = if is_admin {
            "00-rust-admin-probe-00"
        } else {
            "00-rust-public-ingress-00"
        };

        if let Some((status_code, body)) = router.route(method, path, trace_id) {
            let status_text = match status_code {
                200 => "OK",
                503 => "Service Unavailable",
                _ => "Error",
            };
            let response = format!(
                "HTTP/1.1 {} {}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
                status_code,
                status_text,
                body.len(),
                body
            );
            let _ = stream.write_all(response.as_bytes());
            let _ = stream.flush();
            return;
        }

        let (status_code, status_text, body) = if is_admin {
            (404, "Not Found", "{\"success\":false,\"statusCode\":404,\"data\":null,\"error\":{\"code\":\"NOT_FOUND\",\"message\":\"Not Found\"},\"meta\":{\"timestamp\":\"\",\"traceId\":\"00-rust-admin-404\"}}".to_string())
        } else {
            let json = format!("{{\"success\":true,\"statusCode\":200,\"data\":{{\"service\":\"{}\",\"status\":\"RUNNING\"}},\"error\":null,\"meta\":{{\"timestamp\":\"\",\"traceId\":\"00-rust-public-root\"}}}}", SERVICE_NAME);
            (200, "OK", json)
        };

        let response = format!(
            "HTTP/1.1 {} {}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{}",
            status_code,
            status_text,
            body.len(),
            body
        );
        let _ = stream.write_all(response.as_bytes());
        let _ = stream.flush();
    }
}

fn main() {
    let port = env::var("PORT")
        .ok()
        .and_then(|p| p.parse::<u16>().ok())
        .unwrap_or(DEFAULT_PORT);
    let admin_port = env::var("ADMIN_PORT")
        .ok()
        .and_then(|p| p.parse::<u16>().ok())
        .unwrap_or(DEFAULT_ADMIN_PORT);

    let repository = Arc::new(HealthChecksRepositoryAdapter::new());
    let service = Arc::new(HealthChecksService::new(repository));
    let handler = Arc::new(HealthChecksHandler::new(service));
    let router = Arc::new(HealthChecksRouter::new(handler));

    let admin_addr = format!("{}:{}", DEFAULT_ADMIN_HOST, admin_port);
    let admin_listener = TcpListener::bind(&admin_addr).expect("Failed to bind Rust admin listener");
    println!("Rust Admin server listening on http://{}", admin_addr);

    let public_addr = format!("{}:{}", DEFAULT_HOST, port);
    let public_listener = TcpListener::bind(&public_addr).expect("Failed to bind Rust public listener");
    println!("Rust Public server listening on http://{}", public_addr);

    let admin_router = Arc::clone(&router);
    thread::spawn(move || {
        for stream in admin_listener.incoming() {
            if let Ok(stream) = stream {
                let r = Arc::clone(&admin_router);
                thread::spawn(move || {
                    handle_connection(stream, r, true);
                });
            }
        }
    });

    for stream in public_listener.incoming() {
        if let Ok(stream) = stream {
            let r = Arc::clone(&router);
            thread::spawn(move || {
                handle_connection(stream, r, false);
            });
        }
    }
}
