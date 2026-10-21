import axios, { type AxiosRequestConfig } from 'axios';
import { CORE_API_ORIGIN } from '$app/env/public';
import { ApiError } from './errors';

interface APIErrorResponse {
	error?: {
		code?: string;
		message?: string;
		details?: { fields?: Record<string, string> };
	};
}

export async function apiRequest<T = unknown>(
	path: `/api/${string}`,
	options: AxiosRequestConfig = {},
	fallback = 'Request failed'
): Promise<T> {
	try {
		const response = await axios.request<T>({
			...options,
			url: new URL(path, CORE_API_ORIGIN).toString(),
			withCredentials: true
		});
		return response.data;
	} catch (error) {
		if (axios.isCancel(error)) throw error;
		if (!axios.isAxiosError<APIErrorResponse>(error)) throw error;

		if (!error.response) {
			throw new ApiError('Unable to connect to the server. Check your connection and try again.', 0, 'network_error');
		}

		const responseError = error.response.data?.error;
		const fields = responseError?.code === 'validation_failed' ? responseError.details?.fields : undefined;

		throw new ApiError(
			fields?.general || responseError?.message || fallback,
			error.response.status,
			responseError?.code || 'http_error'
		);
	}
}
