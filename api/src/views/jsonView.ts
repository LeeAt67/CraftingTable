export function ok<T>(data: T) {
  return { data };
}

type ErrorMetadata = {
  kind?: string;
  retryable?: boolean;
  requestId?: string;
  details?: Record<string, unknown>;
};

export function fail(code: string, message: string, metadata: ErrorMetadata = {}) {
  return { error: { code, message, ...metadata } };
}
