import { get } from 'svelte/store';
import { setVaultUnlocked, vaultUnlocked } from './state';
import type { VaultState } from './types';
import { callVaultWorker } from './worker-client';

const VAULT_IDLE_TIMEOUT_MS = 60 * 60 * 1000;

let idleTimer: number | undefined;

if (typeof window !== 'undefined') {
	try {
		window.sessionStorage.removeItem('arkive:vault-session:v1');
	} catch {
		// Ignore unavailable browser storage; the vault no longer depends on it.
	}
}

function clearIdleTimer(): void {
	if (idleTimer !== undefined && typeof window !== 'undefined') window.clearTimeout(idleTimer);
	idleTimer = undefined;
}

function scheduleIdleLock(): void {
	clearIdleTimer();
	if (typeof window === 'undefined') return;
	idleTimer = window.setTimeout(() => {
		void lockVault().catch(() => {});
	}, VAULT_IDLE_TIMEOUT_MS);
}

export function touchSessionUnlock(): void {
	if (get(vaultUnlocked)) scheduleIdleLock();
}

export function waitUntilReady(): Promise<VaultState> {
	return Promise.resolve({ unlocked: get(vaultUnlocked) });
}

export async function unlockVault(exportKey: ArrayBuffer, salt: string, encryptedMasterKey: string): Promise<VaultState> {
	const result = await callVaultWorker('unlockVault', {
		exportKey,
		salt,
		encryptedMasterKey,
		aad: 'arkive:master-key:v1'
	}, [exportKey]);
	setVaultUnlocked(result.unlocked);
	if (result.unlocked) scheduleIdleLock();
	else clearIdleTimer();
	return result;
}

export async function lockVault(): Promise<VaultState> {
	clearIdleTimer();
	try {
		return await callVaultWorker('lockVault', {});
	} finally {
		setVaultUnlocked(false);
	}
}

export async function unlockForLogin(exportKey: ArrayBuffer, salt: string, encryptedMasterKey: string): Promise<void> {
	await unlockVault(exportKey, salt, encryptedMasterKey);
}
