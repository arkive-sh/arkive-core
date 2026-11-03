import { api } from '#lib/api.ts';
import { lockVault, unlockForLogin } from '#lib/vault/session.ts';
import { opaqueAuth } from '#lib/opaque-auth.ts';

export async function loginAndUnlock(email: string, password: string): Promise<void> {
	let login: Awaited<ReturnType<typeof opaqueAuth.login>> | null = null;
	let exportKey: Uint8Array<ArrayBuffer> | null = null;

	try {
		login = await opaqueAuth.login(email, password);
		exportKey = login.exportKey;
		const { salt, encryptedMasterKey } = login;
		login = null;
		await unlockForLogin(exportKey, salt, encryptedMasterKey);
	} catch (error) {
		await lockVault().catch(() => {});

		throw error;
	} finally {
		password = '';
		if (exportKey?.byteLength) exportKey.fill(0);
		exportKey = null;
		login = null;
	}
}

export async function logoutAndLock(): Promise<void> {
	try {
		await lockVault();
	} finally {
		await api.post('/api/auth/logout');
	}
}
