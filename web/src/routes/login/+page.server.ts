import { redirect } from '@sveltejs/kit';
import axios from 'axios';
import { api } from '#lib/api.ts';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ request }) => {
	try {
		await api.get('/api/me', { headers: { Cookie: request.headers.get('cookie') ?? '' } });
	} catch (error) {
		if (axios.isAxiosError(error) && error.response?.status === 401) return {};
		throw error;
	}

	redirect(303, '/');
};
