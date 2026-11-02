use serde::Serialize;

#[derive(Serialize, Clone)]
pub struct ApiResponseEnvelope<T: Serialize> {
    pub success: bool,
    pub status_code: u16,
    pub data: Option<T>,
    pub error: Option<ApiErrorEnvelope>,
    pub meta: ApiMetaEnvelope,
}

#[derive(Serialize, Clone)]
pub struct ApiErrorEnvelope {
    pub code: String,
    pub message: String,
}

#[derive(Serialize, Clone)]
pub struct ApiMetaEnvelope {
    pub timestamp: String,
    pub trace_id: String,
}
