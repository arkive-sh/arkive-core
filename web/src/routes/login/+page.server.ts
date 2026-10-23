import { redirect } from '@sveltejs/kit';
import { api } from '#lib/api.ts';
import type { SetupStatus } from '../setup/types';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ request }) => {
	const headers = { Cookie: request.headers.get('cookie') ?? '' };
	const { data: setup } = await api.get<SetupStatus>('/api/setup/status', { headers });
	if (!setup.initialized) redirect(303, '/setup');
	if (setup.recoveryPending) redirect(303, '/setup/recovery-key');

	return {};
};
