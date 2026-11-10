import { deriveRecoveryProofPublicKey, signRecoveryProof } from '../../recovery-proof';
import { fromBase64, toBase64 } from '../bytes';
import { VAULT_MASTER_KEY_AAD, recoveryMasterKeyAAD } from '../aad';
import { formatRecoveryKey, parseRecoveryKey } from '../recovery-key';
import {
	assertVaultGeneration,
	aadBytes,
	clearBytes,
	decryption,
	encryption,
	hashes,
	lockVault,
	randomBytes,
	toUint8Array,
	workerState,
} from './runtime';
import type { WorkerHandlers, WorkerInput } from './types';

async function deriveVaultKEK(exportKey: Uint8Array, salt: Uint8Array): Promise<Uint8Array> {
	let rawKey: Uint8Array<ArrayBuffer> | null = new Uint8Array(exportKey);
	let rawSalt: Uint8Array<ArrayBuffer> | null = new Uint8Array(salt);

	try {
		return await hashes.hkdf(
			rawKey,
			rawSalt,
			new TextEncoder().encode('arkive:opaque-vault-kek:v1'),
		);
	} finally {
		rawKey = clearBytes(rawKey);
		rawSalt = clearBytes(rawSalt);
	}
}

async function wrapRecoveryMasterKey(
	masterKey: Uint8Array,
	recoveryKey: Uint8Array,
	userID: string,
): Promise<Uint8Array> {
	let kek: Uint8Array | null = null;

	try {
		kek = await hashes.hkdf(
			recoveryKey,
			new Uint8Array(32),
			new TextEncoder().encode('arkive-recovery-kek-v1'),
		);

		return await encryption.wrapKey(masterKey, kek, aadBytes(recoveryMasterKeyAAD(userID)));
	} finally {
		kek = clearBytes(kek);
	}
}

async function unwrapRecoveryMasterKey(
	encryptedMasterKey: Uint8Array,
	recoveryKey: Uint8Array,
	userID: string,
): Promise<Uint8Array> {
	let kek: Uint8Array | null = null;

	try {
		kek = await hashes.hkdf(
			recoveryKey,
			new Uint8Array(32),
			new TextEncoder().encode('arkive-recovery-kek-v1'),
		);

		return await decryption.unwrapKey(
			encryptedMasterKey,
			kek,
			aadBytes(recoveryMasterKeyAAD(userID)),
		);
	} catch (cause) {
		throw new Error('Invalid recovery key', { cause });
	} finally {
		kek = clearBytes(kek);
	}
}

async function handleUnlockVault(params: WorkerInput<'unlockVault'>) {
	const generation = workerState.vaultGeneration;
	let exportKey;
	let salt;
	let encryptedMasterKey;
	let kek;
	let masterKey;

	try {
		if (!params.exportKey) throw new Error('Missing vault export key');
		exportKey = toUint8Array(params.exportKey);
		params.exportKey = null;
		salt = fromBase64(params.salt);
		encryptedMasterKey = fromBase64(params.encryptedMasterKey);
		kek = await deriveVaultKEK(exportKey, salt);
		masterKey = await decryption.unwrapKey(encryptedMasterKey, kek, aadBytes(params.aad));
		assertVaultGeneration(generation);
		workerState.unlockedMasterKey = masterKey.slice();

		return { unlocked: true };
	} finally {
		exportKey = clearBytes(exportKey);
		salt = clearBytes(salt);
		encryptedMasterKey = clearBytes(encryptedMasterKey);
		kek = clearBytes(kek);
		masterKey = clearBytes(masterKey);
	}
}

async function handleCreateInitialVault(params: WorkerInput<'createInitialVault'>) {
	let exportKey;
	let salt;
	let masterKey;
	let kek;
	let encryptedMasterKey;

	try {
		if (!params.exportKey) throw new Error('Missing vault export key');
		exportKey = toUint8Array(params.exportKey);
		params.exportKey = null;
		salt = randomBytes(16);
		masterKey = randomBytes(32);
		kek = await deriveVaultKEK(exportKey, salt);
		encryptedMasterKey = await encryption.wrapKey(masterKey, kek, aadBytes(VAULT_MASTER_KEY_AAD));

		return {
			vaultSalt: toBase64(salt),
			encryptedMasterKey: toBase64(encryptedMasterKey),
			masterKey: masterKey.slice(),
		};
	} finally {
		exportKey = clearBytes(exportKey);
		salt = clearBytes(salt);
		masterKey = clearBytes(masterKey);
		kek = clearBytes(kek);
		encryptedMasterKey = clearBytes(encryptedMasterKey);
	}
}

async function handlePrepareInitialRecovery(params: WorkerInput<'prepareInitialRecovery'>) {
	const userID = String(params.userID || '').trim();

	if (!userID) throw new Error('Recovery setup context is unavailable');

	let masterKey;
	let recoveryKey;
	let encryptedMasterKeyRecovery;
	let recoveryProofPublicKey;

	try {
		if (!params.masterKeyBytes) throw new Error('Setup master key is unavailable');
		masterKey = toUint8Array(params.masterKeyBytes);
		params.masterKeyBytes = null;
		if (!masterKey.length) throw new Error('Setup master key is unavailable');
		recoveryKey = randomBytes(32);
		encryptedMasterKeyRecovery = await wrapRecoveryMasterKey(masterKey, recoveryKey, userID);
		recoveryProofPublicKey = await deriveRecoveryProofPublicKey(recoveryKey, userID);

		return {
			recoveryKey: await formatRecoveryKey(recoveryKey, hashes),
			encryptedMasterKeyRecovery: toBase64(encryptedMasterKeyRecovery),
			recoveryProofPublicKey: toBase64(recoveryProofPublicKey),
		};
	} finally {
		masterKey = clearBytes(masterKey);
		recoveryKey = clearBytes(recoveryKey);
		encryptedMasterKeyRecovery = clearBytes(encryptedMasterKeyRecovery);
		recoveryProofPublicKey = clearBytes(recoveryProofPublicKey);
	}
}

async function handleLockVault() {
	lockVault();

	return { unlocked: false };
}

async function handlePreparePasswordReset(params: WorkerInput<'preparePasswordReset'>) {
	let exportKey;
	let encryptedMasterKeyRecovery;
	let recoveryKeyBytes;
	let recoveryKeyText: string | null = null;
	let recoveryKey;
	let salt;
	let masterKey;
	let recoveryProof;
	let kek;
	let wrappedMasterKey;

	try {
		if (!params.exportKey) throw new Error('Missing vault export key');
		exportKey = toUint8Array(params.exportKey);
		params.exportKey = null;
		encryptedMasterKeyRecovery = fromBase64(params.encryptedMasterKeyRecovery);
		if (!params.recoveryKey) throw new Error('Recovery key is unavailable');
		recoveryKeyBytes = toUint8Array(params.recoveryKey);
		params.recoveryKey = null;
		recoveryKeyText = new TextDecoder().decode(recoveryKeyBytes);
		recoveryKeyBytes = clearBytes(recoveryKeyBytes);
		recoveryKey = await parseRecoveryKey(recoveryKeyText, hashes);
		recoveryKeyText = null;
		const userID = String(params.userID || '');
		salt = randomBytes(16);
		masterKey = await unwrapRecoveryMasterKey(encryptedMasterKeyRecovery, recoveryKey, userID);
		recoveryProof = await signRecoveryProof(recoveryKey, userID, String(params.resetToken || ''));
		kek = await deriveVaultKEK(exportKey, salt);
		wrappedMasterKey = await encryption.wrapKey(masterKey, kek, aadBytes(VAULT_MASTER_KEY_AAD));

		return {
			vaultSalt: toBase64(salt),
			encryptedMasterKey: toBase64(wrappedMasterKey),
			recoveryProof: toBase64(recoveryProof),
		};
	} finally {
		exportKey = clearBytes(exportKey);
		encryptedMasterKeyRecovery = clearBytes(encryptedMasterKeyRecovery);
		recoveryKeyBytes = clearBytes(recoveryKeyBytes);
		recoveryKeyText = null;
		recoveryKey = clearBytes(recoveryKey);
		salt = clearBytes(salt);
		masterKey = clearBytes(masterKey);
		recoveryProof = clearBytes(recoveryProof);
		kek = clearBytes(kek);
		wrappedMasterKey = clearBytes(wrappedMasterKey);
	}
}

export const vault_operations = {
	unlockVault: handleUnlockVault,
	createInitialVault: handleCreateInitialVault,
	prepareInitialRecovery: handlePrepareInitialRecovery,
	lockVault: handleLockVault,
	preparePasswordReset: handlePreparePasswordReset,
} satisfies Pick<
	WorkerHandlers,
	| 'unlockVault'
	| 'createInitialVault'
	| 'prepareInitialRecovery'
	| 'lockVault'
	| 'preparePasswordReset'
>;
