import { apiRequest } from '#lib/api.ts';
import type { DashboardResponse } from '#lib/dashboard.ts';
import type { PageServerLoad } from './$types';

export const load: PageServerLoad = async ({ parent, request }) => {
	await parent();
	const dashboard = await apiRequest<DashboardResponse>(
		'/api/dashboard',
		{ headers: { Cookie: request.headers.get('cookie') ?? '' } },
		'Could not load dashboard'
	);
	return { dashboard };
};
