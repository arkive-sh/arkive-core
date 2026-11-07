import { error } from '@sveltejs/kit';
import axios from 'axios';
import { api } from '#lib/api.ts';
import type { PageServerLoad } from './$types';

const SHARE_ACCESS_COOKIE = 'arkive_share_access';

export const load: PageServerLoad = async ({ cookies, params, setHeaders }) => {
	setHeaders({ 'cache-control': 'no-store' });
	const shareAccess = cookies.get(SHARE_ACCESS_COOKIE);
	const headers = shareAccess ? { Cookie: `${SHARE_ACCESS_COOKIE}=${shareAccess}` } : undefined;

	try {
		await api.get(`/api/public/shares/${encodeURIComponent(params.token)}`, {
			headers,
		});
	} catch (cause) {
		if (!axios.isAxiosError(cause)) throw cause;
		if (cause.response?.status === 404 || cause.response?.status === 410)
			error(404, { message: 'Not found' });
	}

	return {};
};
