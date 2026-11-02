use std::fmt;

#[derive(Debug)]
pub struct ApplicationError {
    pub message: String,
    pub status_code: u16,
    pub code: String,
}

impl ApplicationError {
    pub fn new(message: &str, status_code: u16, code: &str) -> Self {
        Self {
            message: message.to_string(),
            status_code,
            code: code.to_string(),
        }
    }
}

impl fmt::Display for ApplicationError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}: {}", self.code, self.message)
    }
}

impl std::error::Error for ApplicationError {}
