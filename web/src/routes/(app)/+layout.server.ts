import { redirect } from '@sveltejs/kit';
import axios from 'axios';
import { api } from '#lib/api.ts';
import type { LayoutServerLoad } from './$types';

interface User {
	id: string;
	brand_name: string;
	email: string;
}

export const load: LayoutServerLoad = async ({ request }) => {
	try {
		const { data: user } = await api.get<User>('/api/me', {
			headers: { Cookie: request.headers.get('cookie') ?? '' }
		});
		return { user };
	} catch (error) {
		if (axios.isAxiosError(error) && error.response?.status === 401) redirect(303, '/login');
		throw error;
	}
};
