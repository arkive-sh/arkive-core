import { redirect } from '@sveltejs/kit';
import axios from 'axios';
import { api } from '#lib/api.ts';
import type { LayoutServerLoad } from './$types';

export const load: LayoutServerLoad = async ({ request, url }) => {
	if (url.pathname !== '/login') return {};

	try {
		await api.get('/api/me', {
			headers: { Cookie: request.headers.get('cookie') ?? '' }
		});
	} catch (error) {
		if (axios.isAxiosError(error) && error.response?.status === 401) return {};
		throw error;
	}

	redirect(303, '/');
};
