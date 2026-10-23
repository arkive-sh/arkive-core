import axios from 'axios';
import { CORE_API_ORIGIN } from '$app/env/public';

export interface APIErrorResponse {
	error?: {
		message?: string;
		details?: { fields?: Record<string, string> };
	};
}

export const api = axios.create({
	baseURL: CORE_API_ORIGIN,
	withCredentials: true
});

api.interceptors.response.use(undefined, (error: unknown) => {
	if (axios.isAxiosError<APIErrorResponse>(error)) {
		const responseError = error.response?.data?.error;
		const message = responseError?.details?.fields?.general || responseError?.message;
		if (message) error.message = message;
		else if (!error.response) error.message = 'Unable to connect to the server. Check your connection and try again.';
	}
	return Promise.reject(error);
});
