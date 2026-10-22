import { toBase64 } from './bytes';
import { setVaultUnlocked } from './state';
import type { StoredSessionUnlock, VaultState } from './types';
import { callVaultWorker } from './worker-client';

const SESSION_UNLOCK_KEY = 'arkive:vault-session:v1';
const SESSION_UNLOCK_TTL_MS = 60 * 60 * 1000;

let restorePromise: Promise<VaultState> | null = null;
let sessionExpiryTimer: number | undefined;

function clearSessionExpiryTimer(): void {
	if (sessionExpiryTimer !== undefined && typeof window !== 'undefined') {
		window.clearTimeout(sessionExpiryTimer);
	}
	sessionExpiryTimer = undefined;
}

function scheduleSessionExpiry(expiresAt: number): void {
	clearSessionExpiryTimer();
	if (typeof window === 'undefined' || !Number.isFinite(expiresAt) || expiresAt <= Date.now()) return;
	sessionExpiryTimer = window.setTimeout(() => {
		clearSessionUnlock();
		setVaultUnlocked(false);
		void callVaultWorker('lockVault', {}).catch(() => {});
	}, expiresAt - Date.now());
}

function loadSessionUnlock(): StoredSessionUnlock | null {
	try {
		const raw = sessionStorage.getItem(SESSION_UNLOCK_KEY);
		if (!raw) return null;
		const parsed = JSON.parse(raw) as Partial<StoredSessionUnlock> | null;
		if (
			!parsed ||
			typeof parsed.sessionUnwrapKey !== 'string' ||
			typeof parsed.wrappedMasterKey !== 'string' ||
			typeof parsed.expiresAt !== 'number'
		) return null;
		if (parsed.expiresAt <= Date.now()) {
			clearSessionUnlock();
			return null;
		}
		scheduleSessionExpiry(parsed.expiresAt);
		return parsed as StoredSessionUnlock;
	} catch {
		return null;
	}
}

function storeSessionUnlock(sessionUnwrapKey: string, wrappedMasterKey: string): StoredSessionUnlock {
	const session = {
		sessionUnwrapKey,
		wrappedMasterKey,
		expiresAt: Date.now() + SESSION_UNLOCK_TTL_MS
	};
	try {
		sessionStorage.setItem(SESSION_UNLOCK_KEY, JSON.stringify(session));
		scheduleSessionExpiry(session.expiresAt);
	} catch {
		// The in-memory vault remains unlocked when session storage is unavailable.
	}
	return session;
}

export function clearSessionUnlock(): void {
	clearSessionExpiryTimer();
	try {
		sessionStorage.removeItem(SESSION_UNLOCK_KEY);
	} catch {
		// Clearing the vault does not depend on session storage being available.
	}
}

async function createSessionUnlock(): Promise<Pick<StoredSessionUnlock, 'sessionUnwrapKey' | 'wrappedMasterKey'>> {
	const sessionUnwrapKey = new Uint8Array(32);
	window.crypto.getRandomValues(sessionUnwrapKey);
	const encodedSessionKey = toBase64(sessionUnwrapKey);
	const transferKey = new Uint8Array(sessionUnwrapKey);
	try {
		const result = await callVaultWorker(
			'createSessionUnlock',
			{ sessionUnwrapKey: transferKey.buffer, aad: 'arkive:session-master-key:v1' },
			[transferKey.buffer]
		);
		return { sessionUnwrapKey: encodedSessionKey, wrappedMasterKey: result.wrappedMasterKey };
	} finally {
		sessionUnwrapKey.fill(0);
		if (transferKey.byteLength) transferKey.fill(0);
	}
}

async function restoreSessionUnlock(): Promise<VaultState> {
	const stored = loadSessionUnlock();
	if (!stored) {
		setVaultUnlocked(false);
		return { unlocked: false };
	}
	try {
		const result = await callVaultWorker('restoreSessionUnlock', {
			sessionUnwrapKey: stored.sessionUnwrapKey,
			wrappedMasterKey: stored.wrappedMasterKey,
			aad: 'arkive:session-master-key:v1'
		});
		setVaultUnlocked(result.unlocked);
		if (result.unlocked) storeSessionUnlock(stored.sessionUnwrapKey, stored.wrappedMasterKey);
		return result;
	} catch (error) {
		clearSessionUnlock();
		setVaultUnlocked(false);
		throw error;
	}
}

export function touchSessionUnlock(): void {
	const stored = loadSessionUnlock();
	if (stored) storeSessionUnlock(stored.sessionUnwrapKey, stored.wrappedMasterKey);
}

export function waitUntilReady(): Promise<VaultState> {
	if (!restorePromise) restorePromise = restoreSessionUnlock().catch(() => ({ unlocked: false }));
	return restorePromise;
}

export async function unlockVault(password: string, salt: string, encryptedMasterKey: string): Promise<VaultState> {
	const result = await callVaultWorker('unlockVault', {
		password,
		salt,
		encryptedMasterKey,
		aad: 'arkive:master-key:v1'
	});
	setVaultUnlocked(result.unlocked);
	restorePromise = Promise.resolve(result);
	return result;
}

export async function persistSessionUnlock(): Promise<Pick<StoredSessionUnlock, 'sessionUnwrapKey' | 'wrappedMasterKey'>> {
	const session = await createSessionUnlock();
	storeSessionUnlock(session.sessionUnwrapKey, session.wrappedMasterKey);
	restorePromise = Promise.resolve({ unlocked: true });
	return session;
}

export async function lockVault(): Promise<VaultState> {
	clearSessionUnlock();
	const result = await callVaultWorker('lockVault', {});
	setVaultUnlocked(false);
	return result;
}

export async function unlockForLogin(password: string, salt: string, encryptedMasterKey: string): Promise<void> {
	await unlockVault(password, salt, encryptedMasterKey);
	await persistSessionUnlock();
}
