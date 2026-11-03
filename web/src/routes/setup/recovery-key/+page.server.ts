import { redirect } from '@sveltejs/kit';
import { api } from '#lib/api.ts';
import type { PageServerLoad } from './$types';
import type { SetupStatus } from '../types';

export const load: PageServerLoad = async ({ request }) => {
	const { data: status } = await api.get<SetupStatus>('/api/setup/status', {
		headers: { Cookie: request.headers.get('cookie') ?? '' },
	});

	if (!status.initialized) redirect(303, '/setup');
	if (!status.recoveryPending || !status.userID) redirect(303, '/login');

	return { userID: status.userID };
};
