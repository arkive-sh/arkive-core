import { get } from 'svelte/store';
import { setVaultUnlocked, vaultUnlocked } from './state';
import type { VaultState } from './types';
import { callVaultWorker } from './worker-client';

if (typeof window !== 'undefined') {
	try {
		window.sessionStorage.removeItem('arkive:vault-session:v1');
	} catch {
		// Ignore unavailable browser storage; the vault no longer depends on it.
	}
}

export function waitUntilReady(): Promise<VaultState> {
	return Promise.resolve({ unlocked: get(vaultUnlocked) });
}

export async function unlockVault(
	exportKey: Uint8Array<ArrayBuffer>,
	salt: string,
	encryptedMasterKey: string,
): Promise<VaultState> {
	const result = await callVaultWorker(
		'unlockVault',
		{
			exportKey: exportKey.buffer,
			salt,
			encryptedMasterKey,
			aad: 'arkive:master-key:v1',
		},
		[exportKey.buffer],
	);
	setVaultUnlocked(result.unlocked);

	return result;
}

export async function lockVault(): Promise<VaultState> {
	try {
		return await callVaultWorker('lockVault', {});
	} finally {
		setVaultUnlocked(false);
	}
}

export async function unlockForLogin(
	exportKey: Uint8Array<ArrayBuffer>,
	salt: string,
	encryptedMasterKey: string,
): Promise<void> {
	await unlockVault(exportKey, salt, encryptedMasterKey);
}
