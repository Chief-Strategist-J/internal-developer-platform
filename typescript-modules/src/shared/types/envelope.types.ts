export interface ApiResponseEnvelope<T> {
  success: boolean;
  statusCode: number;
  data: T | null;
  error: ApiErrorEnvelope | null;
  meta: ApiMetaEnvelope;
}

export interface ApiErrorEnvelope {
  code: string;
  message: string;
  details?: Record<string, unknown>;
}

export interface ApiMetaEnvelope {
  timestamp: string;
  traceId: string;
}
