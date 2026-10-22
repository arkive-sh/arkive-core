import { redirect } from '@sveltejs/kit';
import { apiRequest } from '#lib/api.ts';
import { ApiError } from '#lib/errors.ts';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ request }) => {
	try {
		await apiRequest('/api/me', { headers: { Cookie: request.headers.get('cookie') ?? '' } });
	} catch (error) {
		if (error instanceof ApiError && error.status === 401) return {};
		throw error;
	}

	redirect(303, '/');
};
