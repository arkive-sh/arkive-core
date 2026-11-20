import { api } from '#lib/api.ts';
import { lockVault, unlockForLogin } from '#lib/vault/session.ts';
import { opaqueAuth, type MFAChallenge, type MFAProof } from '#lib/opaque-auth.ts';

export async function loginAndUnlock(
	email: string,
	password: string,
	requestMFA: (challenge: MFAChallenge, invalidCode: boolean) => Promise<MFAProof>,
): Promise<string[]> {
	let login: Awaited<ReturnType<typeof opaqueAuth.login>> | null = null;
	let exportKey: Uint8Array<ArrayBuffer> | null = null;

	try {
		login = await opaqueAuth.login(email, password, requestMFA);
		exportKey = login.exportKey;
		const { userID, salt, encryptedMasterKey, recoveryCodes = [] } = login;
		login = null;
		await unlockForLogin(exportKey, salt, encryptedMasterKey, userID);

		return recoveryCodes;
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
