import { get } from 'svelte/store';
import { VAULT_MASTER_KEY_AAD } from './aad';
import { fromBase64, toBase64 } from './bytes';
import { setVaultUnlocked, vaultUnlocked } from './state';
import type { VaultState } from './types';
import { callVaultWorker } from './worker-client';

const VAULT_SESSION_STORAGE_KEY = 'arkive:vault-session:v1';
const KEEP_VAULT_UNLOCKED_KEY = 'arkive.vault.keep-unlocked';

export function shouldKeepVaultUnlocked(): boolean {
	try {
		return localStorage.getItem(KEEP_VAULT_UNLOCKED_KEY) === 'true';
	} catch {
		return false;
	}
}

function clearSavedVaultSession(): void {
	try {
		sessionStorage.removeItem(VAULT_SESSION_STORAGE_KEY);
	} catch {
		// The vault still locks in memory if browser storage is unavailable.
	}
}

async function saveVaultSession(userID: string): Promise<void> {
	const { masterKey } = await callVaultWorker('exportVaultSession', {});

	try {
		sessionStorage.setItem(
			VAULT_SESSION_STORAGE_KEY,
			JSON.stringify({ userID, masterKey: toBase64(masterKey) }),
		);
	} finally {
		masterKey.fill(0);
	}
}

export async function setKeepVaultUnlocked(enabled: boolean, userID: string): Promise<void> {
	if (!enabled) {
		try {
			localStorage.removeItem(KEEP_VAULT_UNLOCKED_KEY);
		} catch {
			// Removing the session key still disables this tab's saved unlock.
		}
		clearSavedVaultSession();

		return;
	}

	try {
		localStorage.setItem(KEEP_VAULT_UNLOCKED_KEY, 'true');
		if (get(vaultUnlocked)) await saveVaultSession(userID);
	} catch {
		try {
			localStorage.removeItem(KEEP_VAULT_UNLOCKED_KEY);
		} catch {
			// Keep the current vault unlocked even if browser storage is unavailable.
		}
		clearSavedVaultSession();

		throw new Error('Could not save the vault session in this browser tab.');
	}
}

async function restoreVaultSession(userID: string): Promise<VaultState> {
	let masterKey: Uint8Array<ArrayBuffer> | null = null;

	try {
		if (!shouldKeepVaultUnlocked()) return { unlocked: false };
		const stored = sessionStorage.getItem(VAULT_SESSION_STORAGE_KEY);

		if (!stored) return { unlocked: false };
		const session = JSON.parse(stored) as { userID?: unknown; masterKey?: unknown };

		if (session.userID !== userID || typeof session.masterKey !== 'string')
			throw new Error('Vault session belongs to another account.');

		masterKey = fromBase64(session.masterKey);
		if (masterKey.byteLength !== 32) throw new Error('Saved vault session is invalid.');
		const masterKeyBuffer = masterKey.buffer;
		masterKey = null;
		const result = await callVaultWorker(
			'restoreVaultSession',
			{ masterKeyBytes: masterKeyBuffer },
			[masterKeyBuffer],
		);
		setVaultUnlocked(result.unlocked);

		return result;
	} catch {
		clearSavedVaultSession();

		return { unlocked: false };
	} finally {
		masterKey?.fill(0);
	}
}

export async function waitUntilReady(userID?: string): Promise<VaultState> {
	if (get(vaultUnlocked)) return { unlocked: true };
	if (!userID) return { unlocked: false };

	return restoreVaultSession(userID);
}

export async function unlockVault(
	exportKey: Uint8Array<ArrayBuffer>,
	salt: string,
	encryptedMasterKey: string,
	userID: string,
): Promise<VaultState> {
	const result = await callVaultWorker(
		'unlockVault',
		{
			exportKey: exportKey.buffer,
			salt,
			encryptedMasterKey,
			aad: VAULT_MASTER_KEY_AAD,
		},
		[exportKey.buffer],
	);
	setVaultUnlocked(result.unlocked);
	if (result.unlocked && shouldKeepVaultUnlocked()) {
		await saveVaultSession(userID).catch(clearSavedVaultSession);
	}

	return result;
}

export async function lockVault(): Promise<VaultState> {
	try {
		return await callVaultWorker('lockVault', {});
	} finally {
		setVaultUnlocked(false);
		clearSavedVaultSession();
	}
}

export async function unlockForLogin(
	exportKey: Uint8Array<ArrayBuffer>,
	salt: string,
	encryptedMasterKey: string,
	userID: string,
): Promise<void> {
	await unlockVault(exportKey, salt, encryptedMasterKey, userID);
}
