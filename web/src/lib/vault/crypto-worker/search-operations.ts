import { fromBase64, toBase64Url } from '../bytes';
import { aadBytes, clearBytes, decryption, hashes, toUint8Array, workerState } from './runtime';
import type { WorkerHandlers, WorkerInput } from './types';

async function handleCreateSearchTokens(params: WorkerInput<'createSearchTokens'>) {
	const vaultId = String(params.vaultId || '');
	let terms: string[] = [];
	let searchKey;

	try {
		if (!workerState.unlockedMasterKey) throw new Error('Vault is locked');
		terms = Array.isArray(params.terms) ? params.terms : [];
		params.terms = null;
		if (!vaultId || !terms.length) return { tokens: [] };
		searchKey = await hashes.hkdf(
			workerState.unlockedMasterKey,
			new Uint8Array(32),
			new TextEncoder().encode('arkive-search-v1'),
		);
		const tokens = [];

		for (let i = 0; i < terms.length; i += 1) {
			let payload;
			let digest;

			try {
				payload = new TextEncoder().encode(vaultId + ':' + String(terms[i] || ''));
				digest = await hashes.hmacSha256(searchKey, payload);
				tokens.push(toBase64Url(digest));
			} finally {
				payload = clearBytes(payload);
				digest = clearBytes(digest);
			}
		}

		return { tokens: tokens };
	} finally {
		terms.length = 0;
		params.terms = null;
		searchKey = clearBytes(searchKey);
	}
}

async function handleDecryptSearchFile(params: WorkerInput<'decryptSearchFile'>) {
	if (!workerState.unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	let encryptedFileKey;
	let encryptedMetadata;
	let fileKey;
	let metadata;

	try {
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			workerState.unlockedMasterKey,
			aadBytes(params.fileKeyAad),
		);
		metadata = await decryption.decrypt(encryptedMetadata, fileKey, aadBytes(params.metadataAad));

		return { metadataBytes: metadata.slice() };
	} finally {
		fileKey = clearBytes(fileKey);
		encryptedFileKey = clearBytes(encryptedFileKey);
		encryptedMetadata = clearBytes(encryptedMetadata);
		metadata = clearBytes(metadata);
	}
}

async function handleDecryptSearchFolder(params: WorkerInput<'decryptSearchFolder'>) {
	if (!workerState.unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	let encryptedName;
	let encryptedMetadata;
	let name;
	let metadata;

	try {
		encryptedName = fromBase64(params.encryptedName);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		name = await decryption.decrypt(
			encryptedName,
			workerState.unlockedMasterKey,
			aadBytes(params.nameAad),
		);
		metadata = await decryption.decrypt(
			encryptedMetadata,
			workerState.unlockedMasterKey,
			aadBytes(params.metadataAad),
		);

		return { nameBytes: name.slice(), metadataBytes: metadata.slice() };
	} finally {
		encryptedName = clearBytes(encryptedName);
		encryptedMetadata = clearBytes(encryptedMetadata);
		name = clearBytes(name);
		metadata = clearBytes(metadata);
	}
}

async function handleDecryptSearchThumbnail(params: WorkerInput<'decryptSearchThumbnail'>) {
	if (!workerState.unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	let encryptedFileKey;
	let encryptedThumbnail;
	let fileKey;
	let thumbnail;

	try {
		if (!params.encryptedThumbnail) throw new Error('Encrypted thumbnail is unavailable');
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		encryptedThumbnail = toUint8Array(params.encryptedThumbnail);
		params.encryptedThumbnail = null;
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			workerState.unlockedMasterKey,
			aadBytes(params.fileKeyAad),
		);
		thumbnail = await decryption.decrypt(
			encryptedThumbnail,
			fileKey,
			aadBytes(params.thumbnailAad),
		);

		return { thumbnailBytes: thumbnail.slice() };
	} finally {
		encryptedFileKey = clearBytes(encryptedFileKey);
		encryptedThumbnail = clearBytes(encryptedThumbnail);
		fileKey = clearBytes(fileKey);
		thumbnail = clearBytes(thumbnail);
	}
}

export const search_operations = {
	createSearchTokens: handleCreateSearchTokens,
	decryptSearchFile: handleDecryptSearchFile,
	decryptSearchFolder: handleDecryptSearchFolder,
	decryptSearchThumbnail: handleDecryptSearchThumbnail,
} satisfies Pick<
	WorkerHandlers,
	'createSearchTokens' | 'decryptSearchFile' | 'decryptSearchFolder' | 'decryptSearchThumbnail'
>;
