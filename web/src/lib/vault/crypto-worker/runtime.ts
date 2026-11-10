import { DecryptionService } from '../decryption-service';
import { EncryptionService } from '../encryption-service';
import { HashService } from '../hash-service';

export type ActiveUpload = { fileKey: Uint8Array | null };

export const workerState = {
	unlockedMasterKey: null as Uint8Array | null,
	vaultGeneration: 0,
	activeUploads: new Map<string, ActiveUpload>(),
	activeReaders: new Map<string, Uint8Array | null>(),
	activeShareReaders: new Map<string, Uint8Array | null>(),
};

export const encryption = new EncryptionService();
export const decryption = new DecryptionService();
export const hashes = new HashService();

export function clearBytes(bytes: Uint8Array | null | undefined): null {
	if (bytes) {
		try {
			bytes.fill(0);
		} catch {
			// A detached buffer no longer exposes its contents to this worker.
		}
	}

	return null;
}

export function aadBytes(value: unknown): Uint8Array {
	// Stable UTF-8 encoding keeps every client deriving the same authenticated context.
	if (!value) return new Uint8Array();
	if (value instanceof Uint8Array) return value;

	return new TextEncoder().encode(String(value));
}

export function randomBytes(length: number): Uint8Array<ArrayBuffer> {
	return globalThis.crypto.getRandomValues(new Uint8Array(length));
}

export function toUint8Array(value: ArrayBuffer | Uint8Array): Uint8Array {
	return value instanceof Uint8Array ? value : new Uint8Array(value);
}

export function lockVault(): void {
	// WebCrypto cannot cancel an operation already in progress; invalidate its generation.
	workerState.vaultGeneration += 1;
	workerState.unlockedMasterKey = clearBytes(workerState.unlockedMasterKey);
	workerState.activeUploads.forEach(({ fileKey }) => clearBytes(fileKey));
	workerState.activeUploads.clear();
	workerState.activeReaders.forEach(clearBytes);
	workerState.activeReaders.clear();
	workerState.activeShareReaders.forEach(clearBytes);
	workerState.activeShareReaders.clear();
}

export function assertVaultGeneration(generation: number): void {
	// Prevent in-flight operations from storing or returning keys after a lock.
	if (generation !== workerState.vaultGeneration)
		throw new Error('Vault was locked during this operation.');
}

export function activeMasterKey(): Uint8Array {
	// Operations borrow this buffer; lockVault owns its lifetime and zeroizes it.
	if (!workerState.unlockedMasterKey) throw new Error('Vault is locked');

	return workerState.unlockedMasterKey;
}
