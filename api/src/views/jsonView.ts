export function ok<T>(data: T) {
  return { data };
}

export function fail(code: string, message: string) {
  return { error: { code, message } };
}
