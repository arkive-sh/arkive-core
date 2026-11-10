import { fromBase64, toBase64 } from '../bytes';
import { ENCRYPTION_VERSION } from '../crypto-format';
import {
	fileKeyAAD,
	fileMetadataAAD,
	folderMetadataAAD,
	folderNameAAD,
	shareFileKeyAAD,
	shareFolderMetadataAAD,
	shareFolderNameAAD,
	shareKeyAAD,
} from '../aad';
import {
	assertVaultGeneration,
	aadBytes,
	clearBytes,
	decryption,
	encryption,
	hashes,
	randomBytes,
	toUint8Array,
	workerState,
} from './runtime';
import type { WorkerHandlers, WorkerInput } from './types';

async function handlePrepareShare(params: WorkerInput<'prepareShare'>) {
	if (!workerState.unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	let encryptedFileKey;
	let fileKey;
	let shareKey;
	let encryptedShareKey;
	let encryptedFileKeyForShare;

	try {
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			workerState.unlockedMasterKey,
			aadBytes(params.fileKeyAad),
		);
		shareKey = randomBytes(32);
		encryptedShareKey = await encryption.wrapKey(
			shareKey,
			workerState.unlockedMasterKey,
			aadBytes(params.shareKeyAad),
		);
		encryptedFileKeyForShare = await encryption.wrapKey(
			fileKey,
			shareKey,
			aadBytes(params.shareFileKeyAad),
		);

		return {
			encryptedShareKey: toBase64(encryptedShareKey),
			encryptedFileKeyForShare: toBase64(encryptedFileKeyForShare),
			shareSecret: shareKey.slice(),
			cryptoVersion: ENCRYPTION_VERSION,
		};
	} finally {
		encryptedFileKey = clearBytes(encryptedFileKey);
		fileKey = clearBytes(fileKey);
		shareKey = clearBytes(shareKey);
		encryptedShareKey = clearBytes(encryptedShareKey);
		encryptedFileKeyForShare = clearBytes(encryptedFileKeyForShare);
	}
}

async function handlePrepareFolderShare(params: WorkerInput<'prepareFolderShare'>) {
	if (!workerState.unlockedMasterKey) throw new Error('Vault is locked');
	const generation = workerState.vaultGeneration;
	let encryptedName;
	let encryptedMetadata;
	let name;
	let metadata;
	let shareKey;
	let encryptedShareKey;
	let shareName;
	let shareMetadata;

	try {
		encryptedName = fromBase64(params.encryptedName);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		name = await decryption.decrypt(
			encryptedName,
			workerState.unlockedMasterKey,
			aadBytes(folderNameAAD(params.vaultId, params.folderId, params.parentFolderId)),
		);
		metadata = await decryption.decrypt(
			encryptedMetadata,
			workerState.unlockedMasterKey,
			aadBytes(folderMetadataAAD(params.vaultId, params.folderId, params.parentFolderId)),
		);
		shareKey = randomBytes(32);
		encryptedShareKey = await encryption.wrapKey(
			shareKey,
			workerState.unlockedMasterKey,
			aadBytes(shareKeyAAD(params.token)),
		);
		shareName = await encryption.encrypt(
			name,
			shareKey,
			aadBytes(shareFolderNameAAD(params.folderId, params.token)),
		);
		shareMetadata = await encryption.encrypt(
			metadata,
			shareKey,
			aadBytes(shareFolderMetadataAAD(params.folderId, params.token)),
		);
		assertVaultGeneration(generation);

		return {
			encryptedShareKey: toBase64(encryptedShareKey),
			encryptedFolderName: toBase64(shareName),
			encryptedFolderMetadata: toBase64(shareMetadata),
			shareSecret: shareKey.slice(),
			cryptoVersion: ENCRYPTION_VERSION,
		};
	} finally {
		encryptedName = clearBytes(encryptedName);
		encryptedMetadata = clearBytes(encryptedMetadata);
		name = clearBytes(name);
		metadata = clearBytes(metadata);
		shareKey = clearBytes(shareKey);
		encryptedShareKey = clearBytes(encryptedShareKey);
		shareName = clearBytes(shareName);
		shareMetadata = clearBytes(shareMetadata);
	}
}

async function handleWrapFileKeyForShare(params: WorkerInput<'wrapFileKeyForShare'>) {
	if (!workerState.unlockedMasterKey) throw new Error('Vault is locked');
	let shareSecret;
	let encryptedFileKey;
	let fileKey;
	let wrapped;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			workerState.unlockedMasterKey,
			aadBytes(fileKeyAAD(params.vaultId, params.fileId, params.parentFolderId)),
		);
		wrapped = await encryption.wrapKey(
			fileKey,
			shareSecret,
			aadBytes(shareFileKeyAAD(params.fileId, params.token)),
		);

		return { encryptedFileKeyForShare: toBase64(wrapped) };
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		encryptedFileKey = clearBytes(encryptedFileKey);
		fileKey = clearBytes(fileKey);
		wrapped = clearBytes(wrapped);
	}
}

async function handleWrapUploadFileKeyForShare(params: WorkerInput<'wrapUploadFileKeyForShare'>) {
	const fileKey = workerState.activeUploads.get(String(params.uploadToken || ''))?.fileKey;

	if (!fileKey) throw new Error('Upload context is missing');
	let shareSecret;
	let wrapped;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		wrapped = await encryption.wrapKey(
			fileKey,
			shareSecret,
			aadBytes(shareFileKeyAAD(params.fileId, params.token)),
		);

		return { encryptedFileKeyForShare: toBase64(wrapped) };
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		wrapped = clearBytes(wrapped);
	}
}

async function handleWrapFolderForShare(params: WorkerInput<'wrapFolderForShare'>) {
	if (!workerState.unlockedMasterKey) throw new Error('Vault is locked');
	const generation = workerState.vaultGeneration;
	let shareSecret;
	let encryptedName;
	let encryptedMetadata;
	let name;
	let metadata;
	let wrappedName;
	let wrappedMetadata;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		encryptedName = fromBase64(params.encryptedName);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		name = await decryption.decrypt(
			encryptedName,
			workerState.unlockedMasterKey,
			aadBytes(folderNameAAD(params.vaultId, params.folderId, params.parentFolderId)),
		);
		metadata = await decryption.decrypt(
			encryptedMetadata,
			workerState.unlockedMasterKey,
			aadBytes(folderMetadataAAD(params.vaultId, params.folderId, params.parentFolderId)),
		);
		wrappedName = await encryption.encrypt(
			name,
			shareSecret,
			aadBytes(shareFolderNameAAD(params.folderId, params.token)),
		);
		wrappedMetadata = await encryption.encrypt(
			metadata,
			shareSecret,
			aadBytes(shareFolderMetadataAAD(params.folderId, params.token)),
		);
		assertVaultGeneration(generation);

		return { encryptedName: toBase64(wrappedName), encryptedMetadata: toBase64(wrappedMetadata) };
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		encryptedName = clearBytes(encryptedName);
		encryptedMetadata = clearBytes(encryptedMetadata);
		name = clearBytes(name);
		metadata = clearBytes(metadata);
		wrappedName = clearBytes(wrappedName);
		wrappedMetadata = clearBytes(wrappedMetadata);
	}
}

async function handleOpenPublicShareFolder(params: WorkerInput<'openPublicShareFolder'>) {
	let shareSecret;
	let encryptedName;
	let encryptedMetadata;
	let name;
	let metadata;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		encryptedName = fromBase64(params.encryptedName);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		name = await decryption.decrypt(
			encryptedName,
			shareSecret,
			aadBytes(shareFolderNameAAD(params.folderId, params.token)),
		);
		metadata = await decryption.decrypt(
			encryptedMetadata,
			shareSecret,
			aadBytes(shareFolderMetadataAAD(params.folderId, params.token)),
		);

		return { nameBytes: name.slice(), metadataBytes: metadata.slice() };
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		encryptedName = clearBytes(encryptedName);
		encryptedMetadata = clearBytes(encryptedMetadata);
		name = clearBytes(name);
		metadata = clearBytes(metadata);
	}
}

async function handleOpenPublicShareFileMetadata(
	params: WorkerInput<'openPublicShareFileMetadata'>,
) {
	let shareSecret;
	let encryptedFileKey;
	let encryptedMetadata;
	let fileKey;
	let metadata;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		encryptedFileKey = fromBase64(params.encryptedFileKeyForShare);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			shareSecret,
			aadBytes(shareFileKeyAAD(params.fileId, params.token)),
		);
		metadata = await decryption.decrypt(
			encryptedMetadata,
			fileKey,
			aadBytes(fileMetadataAAD(params.vaultId, params.fileId)),
		);

		return { metadataBytes: metadata.slice() };
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		encryptedFileKey = clearBytes(encryptedFileKey);
		encryptedMetadata = clearBytes(encryptedMetadata);
		fileKey = clearBytes(fileKey);
		metadata = clearBytes(metadata);
	}
}

async function handleOpenShareKey(params: WorkerInput<'openShareKey'>) {
	if (!workerState.unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	let encryptedShareKey;
	let shareKey;

	try {
		encryptedShareKey = fromBase64(params.encryptedShareKey);
		shareKey = await decryption.unwrapKey(
			encryptedShareKey,
			workerState.unlockedMasterKey,
			aadBytes(params.shareKeyAad),
		);

		return { shareSecret: shareKey.slice() };
	} finally {
		encryptedShareKey = clearBytes(encryptedShareKey);
		shareKey = clearBytes(shareKey);
	}
}

async function handleOpenPublicShareContext(params: WorkerInput<'openPublicShareContext'>) {
	const generation = workerState.vaultGeneration;
	const contextID = String(params.contextId || '');
	let shareSecret;
	let encryptedFileKeyForShare;
	let encryptedMetadata;
	let encryptedManifest;
	let fileKey;
	let metadataBytes;
	let manifestBytes;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		if (!contextID) throw new Error('Missing share context');
		encryptedFileKeyForShare = fromBase64(params.encryptedFileKeyForShare);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		encryptedManifest = fromBase64(params.encryptedManifest);
		fileKey = await decryption.unwrapKey(
			encryptedFileKeyForShare,
			shareSecret,
			aadBytes(params.shareFileKeyAad),
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
		workerState.activeShareReaders.set(contextID, fileKey.slice());

		return {
			metadataBytes: metadataBytes.slice(),
			manifestBytes: manifestBytes.slice(),
		};
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		encryptedFileKeyForShare = clearBytes(encryptedFileKeyForShare);
		encryptedMetadata = clearBytes(encryptedMetadata);
		encryptedManifest = clearBytes(encryptedManifest);
		fileKey = clearBytes(fileKey);
		metadataBytes = clearBytes(metadataBytes);
		manifestBytes = clearBytes(manifestBytes);
	}
}

async function handleClosePublicShareContext(params: WorkerInput<'closePublicShareContext'>) {
	const contextID = String(params.contextId || '');
	let fileKey = workerState.activeShareReaders.get(contextID);

	if (fileKey) {
		fileKey = clearBytes(fileKey);
		workerState.activeShareReaders.delete(contextID);
	}

	return { cleared: true };
}

async function handleDecryptPublicShareChunk(params: WorkerInput<'decryptPublicShareChunk'>) {
	const contextID = String(params.contextId || '');
	const fileKey = workerState.activeShareReaders.get(contextID);

	if (!fileKey) {
		throw new Error('Share context is missing');
	}
	let encryptedChunk;
	let actualHash: Uint8Array | null = null;
	let chunkBytes;

	try {
		if (!params.encryptedChunk) throw new Error('Encrypted chunk is unavailable');
		encryptedChunk = toUint8Array(params.encryptedChunk);
		params.encryptedChunk = null;
		const expectedHash = String(params.expectedHash || '');

		if (expectedHash) {
			actualHash = await hashes.sha256(encryptedChunk);
			const hashEncoding = String(params.hashEncoding || 'base64').toLowerCase();
			const actual =
				hashEncoding === 'hex'
					? Array.from(actualHash)
							.map(function (byte) {
								return byte.toString(16).padStart(2, '0');
							})
							.join('')
					: toBase64(actualHash);

			if (actual !== expectedHash) {
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

export const share_operations = {
	prepareShare: handlePrepareShare,
	prepareFolderShare: handlePrepareFolderShare,
	wrapFileKeyForShare: handleWrapFileKeyForShare,
	wrapUploadFileKeyForShare: handleWrapUploadFileKeyForShare,
	wrapFolderForShare: handleWrapFolderForShare,
	openPublicShareFolder: handleOpenPublicShareFolder,
	openPublicShareFileMetadata: handleOpenPublicShareFileMetadata,
	openShareKey: handleOpenShareKey,
	openPublicShareContext: handleOpenPublicShareContext,
	closePublicShareContext: handleClosePublicShareContext,
	decryptPublicShareChunk: handleDecryptPublicShareChunk,
} satisfies Pick<
	WorkerHandlers,
	| 'prepareShare'
	| 'prepareFolderShare'
	| 'wrapFileKeyForShare'
	| 'wrapUploadFileKeyForShare'
	| 'wrapFolderForShare'
	| 'openPublicShareFolder'
	| 'openPublicShareFileMetadata'
	| 'openShareKey'
	| 'openPublicShareContext'
	| 'closePublicShareContext'
	| 'decryptPublicShareChunk'
>;
