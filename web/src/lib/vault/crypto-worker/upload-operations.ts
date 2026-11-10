import { fromBase64, toBase64 } from '../bytes';
import { ENCRYPTION_VERSION } from '../crypto-format';
import {
	assertVaultGeneration,
	aadBytes,
	clearBytes,
	encryption,
	hashes,
	randomBytes,
	toUint8Array,
	workerState,
} from './runtime';
import type { ActiveUpload } from './runtime';
import type { WorkerHandlers, WorkerInput } from './types';

async function handlePrepareUpload(params: WorkerInput<'prepareUpload'>) {
	const uploadToken = String(params.uploadToken || '');

	if (!uploadToken) {
		throw new Error('Missing upload token');
	}
	if (!workerState.unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	const generation = workerState.vaultGeneration;
	let fileKey;
	let encryptedFileKey;

	try {
		fileKey = randomBytes(32);
		encryptedFileKey = await encryption.wrapKey(
			fileKey,
			workerState.unlockedMasterKey,
			aadBytes(params.fileKeyAad),
		);
		assertVaultGeneration(generation);
		workerState.activeUploads.set(uploadToken, { fileKey: fileKey.slice() });

		return {
			encryptedFileKey: toBase64(encryptedFileKey),
			totalParts: Number(params.totalParts || 0),
			encryptionVersion: ENCRYPTION_VERSION,
		};
	} finally {
		fileKey = clearBytes(fileKey);
		encryptedFileKey = clearBytes(encryptedFileKey);
	}
}

async function handleEncryptUploadMetadata(params: WorkerInput<'encryptUploadMetadata'>) {
	const uploadToken = String(params.uploadToken || '');
	let upload: ActiveUpload | undefined;
	let metadata;
	let encryptedMetadata;

	try {
		if (!params.metadataBytes) throw new Error('Metadata is unavailable');
		metadata = toUint8Array(params.metadataBytes);
		params.metadataBytes = null;
		upload = workerState.activeUploads.get(uploadToken);
		if (!upload || !upload.fileKey) throw new Error('Upload context is missing');
		encryptedMetadata = await encryption.encrypt(
			metadata,
			upload.fileKey,
			aadBytes(params.metadataAad),
		);

		return { encryptedMetadata: toBase64(encryptedMetadata) };
	} finally {
		params.metadataBytes = null;
		metadata = clearBytes(metadata);
		encryptedMetadata = clearBytes(encryptedMetadata);
	}
}

async function handleEncryptUploadThumbnail(params: WorkerInput<'encryptUploadThumbnail'>) {
	const uploadToken = String(params.uploadToken || '');
	let upload: ActiveUpload | undefined;
	let thumbnailBytes;
	let encryptedThumbnail;

	try {
		if (!params.thumbnailBytes) throw new Error('Thumbnail data is unavailable');
		thumbnailBytes = toUint8Array(params.thumbnailBytes);
		params.thumbnailBytes = null;
		upload = workerState.activeUploads.get(uploadToken);
		if (!upload || !upload.fileKey) throw new Error('Upload context is missing');
		encryptedThumbnail = await encryption.encrypt(
			thumbnailBytes,
			upload.fileKey,
			aadBytes(params.aad),
		);

		return {
			encryptedThumbnail: encryptedThumbnail.slice(),
			encryptedSize: encryptedThumbnail.length,
		};
	} finally {
		params.thumbnailBytes = null;
		thumbnailBytes = clearBytes(thumbnailBytes);
		encryptedThumbnail = clearBytes(encryptedThumbnail);
	}
}

async function handleEncryptUploadPart(params: WorkerInput<'encryptUploadPart'>) {
	const uploadToken = String(params.uploadToken || '');
	let upload: ActiveUpload | undefined;
	let chunkBytes;
	let encryptedChunk;
	let encryptedHash;

	try {
		if (!params.chunkBytes) throw new Error('Upload data is unavailable');
		chunkBytes = toUint8Array(params.chunkBytes);
		params.chunkBytes = null;
		upload = workerState.activeUploads.get(uploadToken);
		if (!upload || !upload.fileKey) throw new Error('Upload context is missing');
		encryptedChunk = await encryption.encrypt(chunkBytes, upload.fileKey, aadBytes(params.aad));
		encryptedHash = await hashes.sha256(encryptedChunk);

		return {
			encryptedChunk: encryptedChunk.slice(),
			encryptedHash: toBase64(encryptedHash),
			encryptedSize: encryptedChunk.length,
		};
	} finally {
		params.chunkBytes = null;
		chunkBytes = clearBytes(chunkBytes);
		encryptedChunk = clearBytes(encryptedChunk);
		encryptedHash = clearBytes(encryptedHash);
	}
}

async function handleHashUploadPayload(params: WorkerInput<'hashUploadPayload'>) {
	if (!params.bytes) throw new Error('Hash input is unavailable');
	let bytes: Uint8Array | null = toUint8Array(params.bytes);
	params.bytes = null;
	let hash;

	try {
		hash = await hashes.sha256(bytes);

		return { hash: toBase64(hash) };
	} finally {
		bytes = clearBytes(bytes);
		hash = clearBytes(hash);
	}
}

async function handleFinalizeUpload(params: WorkerInput<'finalizeUpload'>) {
	const uploadToken = String(params.uploadToken || '');
	let upload: ActiveUpload | undefined;
	let manifest;
	const decoded: Uint8Array[] = [];
	let encryptedManifest;
	let combined;
	let encryptedHash;

	try {
		upload = workerState.activeUploads.get(uploadToken);
		if (!upload || !upload.fileKey) throw new Error('Upload context is missing');
		if (!params.manifestBytes) throw new Error('Manifest is unavailable');
		manifest = toUint8Array(params.manifestBytes);
		params.manifestBytes = null;
		encryptedManifest = await encryption.encrypt(
			manifest,
			upload.fileKey,
			aadBytes(params.manifestAad),
		);
		const partHashes = params.partHashes || [];
		let totalLength = 0;

		for (let i = 0; i < partHashes.length; i += 1) {
			const hashBytes = fromBase64(partHashes[i]);
			decoded.push(hashBytes);
			totalLength += hashBytes.length;
		}
		combined = new Uint8Array(totalLength);
		let offset = 0;

		for (let i = 0; i < decoded.length; i += 1) {
			combined.set(decoded[i], offset);
			offset += decoded[i].length;
		}
		encryptedHash = await hashes.sha256(combined);

		return {
			encryptedManifest: toBase64(encryptedManifest),
			encryptedHash: toBase64(encryptedHash),
		};
	} finally {
		manifest = clearBytes(manifest);
		encryptedManifest = clearBytes(encryptedManifest);
		while (decoded.length) clearBytes(decoded.pop());
		combined = clearBytes(combined);
		encryptedHash = clearBytes(encryptedHash);
		if (upload) {
			upload.fileKey = clearBytes(upload.fileKey);
			workerState.activeUploads.delete(uploadToken);
		}
		params.manifestBytes = null;
	}
}

async function handleClearUploadContext(params: WorkerInput<'clearUploadContext'>) {
	const uploadToken = String(params.uploadToken || '');
	const upload = workerState.activeUploads.get(uploadToken);

	if (upload) {
		upload.fileKey = clearBytes(upload.fileKey);
		workerState.activeUploads.delete(uploadToken);
	}

	return { cleared: true };
}

export const upload_operations = {
	prepareUpload: handlePrepareUpload,
	encryptUploadMetadata: handleEncryptUploadMetadata,
	encryptUploadThumbnail: handleEncryptUploadThumbnail,
	encryptUploadPart: handleEncryptUploadPart,
	hashUploadPayload: handleHashUploadPayload,
	finalizeUpload: handleFinalizeUpload,
	clearUploadContext: handleClearUploadContext,
} satisfies Pick<
	WorkerHandlers,
	| 'prepareUpload'
	| 'encryptUploadMetadata'
	| 'encryptUploadThumbnail'
	| 'encryptUploadPart'
	| 'hashUploadPayload'
	| 'finalizeUpload'
	| 'clearUploadContext'
>;
