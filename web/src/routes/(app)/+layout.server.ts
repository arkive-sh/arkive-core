import { redirect } from '@sveltejs/kit';
import axios from 'axios';
import { api } from '#lib/api.ts';
import type { LayoutServerLoad } from './$types';
import type { SetupStatus } from '../setup/types';

interface User {
	id: string;
	brand_name: string;
	email: string;
}

export const load: LayoutServerLoad = async ({ request, url }) => {
	const headers = { Cookie: request.headers.get('cookie') ?? '' };
	if (url.pathname === '/') {
		const { data: setup } = await api.get<SetupStatus>('/api/setup/status', { headers });
		if (!setup.initialized) redirect(303, '/setup');
		if (setup.recoveryPending) redirect(303, '/setup/recovery-key');
	}

	try {
		const { data: user } = await api.get<User>('/api/me', {
			headers
		});
		return { user };
	} catch (error) {
		if (axios.isAxiosError(error) && error.response?.status === 401) redirect(303, '/login');
		throw error;
	}
};
