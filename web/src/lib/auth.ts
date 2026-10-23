import { api } from '#lib/api.ts';
import { clearSessionUnlock, lockVault, unlockForLogin } from '#lib/vault/session.ts';

interface LoginResponse {
	salt: string;
	encryptedMasterKey: string;
}

export async function loginAndUnlock(email: string, password: string): Promise<void> {
	const { data: result } = await api.post<LoginResponse>('/api/auth/login', { email, password });

	try {
		await unlockForLogin(password, result.salt, result.encryptedMasterKey);
	} catch (error) {
		await clearSessionUnlock();
		await api.post('/api/auth/logout').catch(() => {});
		throw error;
	}
}

export async function logoutAndLock(): Promise<void> {
	await api.post('/api/auth/logout');
	await lockVault();
}
