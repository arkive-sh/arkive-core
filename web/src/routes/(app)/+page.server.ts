import { api } from '#lib/api.ts';
import type { DashboardResponse } from './dashboard';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ parent, request }) => {
	await parent();
	const { data: dashboard } = await api.get<DashboardResponse>('/api/dashboard', {
		headers: { Cookie: request.headers.get('cookie') ?? '' }
	});
	return { dashboard };
};
