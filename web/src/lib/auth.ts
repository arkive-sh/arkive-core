import { apiRequest } from '#lib/api.ts';
import { clearSessionUnlock, unlockForLogin } from '#lib/vault.ts';

interface LoginResponse {
	salt: string;
	encryptedMasterKey: string;
}

export async function loginAndUnlock(email: string, password: string): Promise<void> {
	const result = await apiRequest<LoginResponse>(
		'/api/auth/login',
		{
			method: 'POST',
			data: { email, password }
		},
		'Unlock failed. Try again.'
	);

	try {
		await unlockForLogin(password, result.salt, result.encryptedMasterKey);
	} catch (error) {
		await clearSessionUnlock();
		await apiRequest('/api/auth/logout', { method: 'POST' }).catch(() => {});
		throw error;
	}
}
