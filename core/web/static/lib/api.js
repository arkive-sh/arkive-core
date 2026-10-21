import { AppError } from "./errors.js";

export const APIError = AppError;

export function parseAPIErrorPayload(data, fallback, status) {
  const payloadError = data?.error;
  const fields = payloadError?.code === "validation_failed" ? payloadError.details?.fields : null;
  const fallbackMessage = String(fallback?.message || fallback || "Request failed");

  return new AppError(fields?.general || payloadError?.message || fallbackMessage, {
    code: payloadError?.code || "http_error",
    details: fields ? { fields } : payloadError?.details || null,
    status: status
  });
}

export async function apiRequest(url, options, fallback) {
  let response;
  try {
    response = await fetch(url, options || {});
  } catch (error) {
    if (error?.name === "AbortError") {
      throw error;
    }
    throw new AppError(fallback?.message || fallback || "Network error", {
      code: "network_error"
    });
  }
  const data = await response.json().catch(() => null);
  if (!response.ok) {
    throw parseAPIErrorPayload(data, fallback, response.status);
  }
  return data;
}
