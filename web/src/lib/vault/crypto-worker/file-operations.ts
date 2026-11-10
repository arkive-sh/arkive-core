import { fromBase64, toBase64 } from '../bytes';
import {
	activeMasterKey,
	assertVaultGeneration,
	aadBytes,
	clearBytes,
	decryption,
	encryption,
	hashes,
	toUint8Array,
	workerState,
} from './runtime';
import type { WorkerHandlers, WorkerInput } from './types';

async function handleRewrapFileKey(params: WorkerInput<'rewrapFileKey'>) {
	if (!workerState.unlockedMasterKey) throw new Error('Vault is locked');
	const generation = workerState.vaultGeneration;
	let encryptedFileKey;
	let fileKey;
	let reboundFileKey;

	try {
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			workerState.unlockedMasterKey,
			aadBytes(params.oldAad),
		);
		assertVaultGeneration(generation);
		reboundFileKey = await encryption.wrapKey(
			fileKey,
			workerState.unlockedMasterKey,
			aadBytes(params.newAad),
		);
		assertVaultGeneration(generation);

		return { encryptedFileKey: toBase64(reboundFileKey) };
	} finally {
		encryptedFileKey = clearBytes(encryptedFileKey);
		fileKey = clearBytes(fileKey);
		reboundFileKey = clearBytes(reboundFileKey);
	}
}

async function handleRewrapFolderMetadata(params: WorkerInput<'rewrapFolderMetadata'>) {
	if (!workerState.unlockedMasterKey) throw new Error('Vault is locked');
	const generation = workerState.vaultGeneration;
	let encryptedName;
	let encryptedMetadata;
	let name;
	let metadata;
	let reboundName;
	let reboundMetadata;

	try {
		encryptedName = fromBase64(params.encryptedName);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		name = await decryption.decrypt(
			encryptedName,
			workerState.unlockedMasterKey,
			aadBytes(params.oldNameAad),
		);
		assertVaultGeneration(generation);
		metadata = await decryption.decrypt(
			encryptedMetadata,
			workerState.unlockedMasterKey,
			aadBytes(params.oldMetadataAad),
		);
		assertVaultGeneration(generation);
		reboundName = await encryption.encrypt(
			name,
			workerState.unlockedMasterKey,
			aadBytes(params.newNameAad),
		);
		assertVaultGeneration(generation);
		reboundMetadata = await encryption.encrypt(
			metadata,
			workerState.unlockedMasterKey,
			aadBytes(params.newMetadataAad),
		);
		assertVaultGeneration(generation);

		return {
			encryptedName: toBase64(reboundName),
			encryptedMetadata: toBase64(reboundMetadata),
		};
	} finally {
		encryptedName = clearBytes(encryptedName);
		encryptedMetadata = clearBytes(encryptedMetadata);
		name = clearBytes(name);
		metadata = clearBytes(metadata);
		reboundName = clearBytes(reboundName);
		reboundMetadata = clearBytes(reboundMetadata);
	}
}

async function handleOpenFileContext(params: WorkerInput<'openFileContext'>) {
	const contextID = String(params.contextId || '');

	if (!contextID) {
		throw new Error('Missing file context');
	}
	if (!workerState.unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	const generation = workerState.vaultGeneration;
	let encryptedFileKey;
	let encryptedMetadata;
	let encryptedManifest;
	let fileKey;
	let metadataBytes;
	let manifestBytes;

	try {
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		encryptedManifest = fromBase64(params.encryptedManifest);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			workerState.unlockedMasterKey,
			aadBytes(params.fileKeyAad),
		);
		assertVaultGeneration(generation);
		metadataBytes = await decryption.decrypt(
			encryptedMetadata,
			fileKey,
			aadBytes(params.metadataAad),
		);
		assertVaultGeneration(generation);
		manifestBytes = await decryption.decrypt(
			encryptedManifest,
			fileKey,
			aadBytes(params.manifestAad),
		);
		assertVaultGeneration(generation);
		workerState.activeReaders.set(contextID, fileKey.slice());

		return {
			metadataBytes: metadataBytes.slice(),
			manifestBytes: manifestBytes.slice(),
		};
	} finally {
		encryptedFileKey = clearBytes(encryptedFileKey);
		encryptedMetadata = clearBytes(encryptedMetadata);
		encryptedManifest = clearBytes(encryptedManifest);
		fileKey = clearBytes(fileKey);
		metadataBytes = clearBytes(metadataBytes);
		manifestBytes = clearBytes(manifestBytes);
	}
}

async function handleCloseFileContext(params: WorkerInput<'closeFileContext'>) {
	const contextID = String(params.contextId || '');
	let fileKey = workerState.activeReaders.get(contextID);

	if (fileKey) {
		fileKey = clearBytes(fileKey);
		workerState.activeReaders.delete(contextID);
	}

	return { cleared: true };
}

async function handleDecryptFileChunk(params: WorkerInput<'decryptFileChunk'>) {
	const contextID = String(params.contextId || '');
	const fileKey = workerState.activeReaders.get(contextID);

	if (!fileKey) {
		throw new Error('File context is missing');
	}
	let encryptedChunk;
	let actualHash;
	let chunkBytes;

	try {
		if (!params.encryptedChunk) throw new Error('Encrypted chunk is unavailable');
		encryptedChunk = toUint8Array(params.encryptedChunk);
		params.encryptedChunk = null;
		const expectedHash = String(params.expectedHash || '');

		if (expectedHash) {
			actualHash = await hashes.sha256(encryptedChunk);
			if (toBase64(actualHash) !== expectedHash) {
				throw new Error('Encrypted chunk hash mismatch');
			}
		}
		chunkBytes = await decryption.decrypt(encryptedChunk, fileKey, aadBytes(params.aad));

		return { chunkBytes: chunkBytes.slice() };
	} finally {
		chunkBytes = clearBytes(chunkBytes);
		encryptedChunk = clearBytes(encryptedChunk);
		actualHash = clearBytes(actualHash);
	}
}

async function handleEncryptFileMetadataInContext(
	params: WorkerInput<'encryptFileMetadataInContext'>,
) {
	const contextID = String(params.contextId || '');
	let fileKey: Uint8Array | null = null;
	let metadata;
	let encrypted;

	try {
		if (!params.metadataBytes) throw new Error('Metadata is unavailable');
		metadata = toUint8Array(params.metadataBytes);
		params.metadataBytes = null;
		fileKey = workerState.activeReaders.get(contextID) ?? null;
		if (!fileKey) throw new Error('File context is missing');
		encrypted = await encryption.encrypt(metadata, fileKey, aadBytes(params.aad));

		return { encryptedMetadata: toBase64(encrypted) };
	} finally {
		metadata = clearBytes(metadata);
		encrypted = clearBytes(encrypted);
		params.metadataBytes = null;
	}
}

async function handleEncryptFileMetadata(params: WorkerInput<'encryptFileMetadata'>) {
	let master: Uint8Array | null = null;
	let metadata;
	let encrypted;

	try {
		if (!params.metadataBytes) throw new Error('Metadata is unavailable');
		metadata = toUint8Array(params.metadataBytes);
		params.metadataBytes = null;
		master = workerState.unlockedMasterKey;
		if (!master) throw new Error('Vault is locked');
		encrypted = await encryption.encrypt(metadata, master, aadBytes(params.aad));

		return { encryptedMetadata: toBase64(encrypted) };
	} finally {
		metadata = clearBytes(metadata);
		encrypted = clearBytes(encrypted);
		params.metadataBytes = null;
	}
}

async function handleDecryptFileMetadata(params: WorkerInput<'decryptFileMetadata'>) {
	const master = activeMasterKey();
	let encryptedMetadata;
	let metadata;

	try {
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		metadata = await decryption.decrypt(encryptedMetadata, master, aadBytes(params.aad));

		return { metadataBytes: metadata.slice() };
	} finally {
		encryptedMetadata = clearBytes(encryptedMetadata);
		metadata = clearBytes(metadata);
	}
}

export const file_operations = {
	rewrapFileKey: handleRewrapFileKey,
	rewrapFolderMetadata: handleRewrapFolderMetadata,
	openFileContext: handleOpenFileContext,
	closeFileContext: handleCloseFileContext,
	decryptFileChunk: handleDecryptFileChunk,
	encryptFileMetadataInContext: handleEncryptFileMetadataInContext,
	encryptFileMetadata: handleEncryptFileMetadata,
	decryptFileMetadata: handleDecryptFileMetadata,
	encryptFolderMetadata: handleEncryptFileMetadata,
	decryptFolderMetadata: handleDecryptFileMetadata,
} satisfies Pick<
	WorkerHandlers,
	| 'rewrapFileKey'
	| 'rewrapFolderMetadata'
	| 'openFileContext'
	| 'closeFileContext'
	| 'decryptFileChunk'
	| 'encryptFileMetadataInContext'
	| 'encryptFileMetadata'
	| 'decryptFileMetadata'
	| 'encryptFolderMetadata'
	| 'decryptFolderMetadata'
>;
