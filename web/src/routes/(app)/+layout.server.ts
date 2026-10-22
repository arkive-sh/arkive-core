import { redirect } from '@sveltejs/kit';
import { apiRequest } from '#lib/api.ts';
import { ApiError } from '#lib/errors.ts';
import type { LayoutServerLoad } from './$types';

interface User {
	id: string;
	brand_name: string;
	email: string;
}

export const load: LayoutServerLoad = async ({ request }) => {
	try {
		const user = await apiRequest<User>(
			'/api/me',
			{ headers: { Cookie: request.headers.get('cookie') ?? '' } },
			'Could not load your account'
		);
		return { user };
	} catch (error) {
		if (error instanceof ApiError && error.status === 401) redirect(303, '/login');
		throw error;
	}
};
