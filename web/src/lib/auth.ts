import { api } from '#lib/api.ts';
import { lockVault, unlockForLogin } from '#lib/vault/session.ts';
import * as opaque from '@serenity-kit/opaque';
import { fromBase64Url } from '#lib/vault/bytes.ts';

interface LoginStartResponse {
	attemptId: string;
	loginResponse: string;
}

interface LoginFinishResponse {
	salt: string;
	encryptedMasterKey: string;
}

export async function loginAndUnlock(email: string, password: string): Promise<void> {
	const { clientLoginState, startLoginRequest } = opaque.client.startLogin({ password });
	const { data: start } = await api.post<LoginStartResponse>('/api/auth/opaque/login/start', { email, startLoginRequest });
	let login: ReturnType<typeof opaque.client.finishLogin>;
	try {
		login = opaque.client.finishLogin({ clientLoginState, loginResponse: start.loginResponse, password });
	} catch {
		throw new Error('Invalid email or password');
	}
	if (!login) throw new Error('Invalid email or password');

	const exportKey = new Uint8Array(fromBase64Url(login.exportKey));
	try {
		const { data: result } = await api.post<LoginFinishResponse>('/api/auth/opaque/login/finish', {
			attemptId: start.attemptId,
			finishLoginRequest: login.finishLoginRequest
		});
		await unlockForLogin(exportKey.buffer, result.salt, result.encryptedMasterKey);
	} catch (error) {
		await lockVault().catch(() => {});
		throw error;
	} finally {
		if (exportKey.buffer.byteLength) exportKey.fill(0);
	}
}

export async function logoutAndLock(): Promise<void> {
	await api.post('/api/auth/logout');
	await lockVault();
}
