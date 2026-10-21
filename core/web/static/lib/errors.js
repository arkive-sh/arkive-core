export class AppError extends Error {
  constructor(message, options) {
    const opts = options || {};
    super(String(message || "Something went wrong"));
    this.name = String(opts.name || "AppError");
    this.code = String(opts.code || "unknown_error");
    this.details = opts.details || null;
    this.status = Number.isFinite(opts.status) ? opts.status : 0;
  }
}

export function toAppError(error, fallback) {
  const base = fallback || {};
  if (error instanceof AppError || (error instanceof Error && error.name === "AbortError")) {
    return error;
  }
  return new AppError((error instanceof Error && error.message) || base.message || "Something went wrong", {
    code: base.code || "unknown_error",
    details: error ? { cause: error } : null,
  });
}
