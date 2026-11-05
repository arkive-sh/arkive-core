import { encodeJSON, normalizeAAD, toWorkerBytes } from './bytes';
import { fileKeyAAD, fileManifestAAD, fileMetadataAAD } from './aad';
import type { VaultAAD, VaultBytes, VaultMetadata, VaultWorkerCalls } from './types';
import { callVaultWorker } from './worker-client';

export function prepareUpload(
	uploadToken: string,
	vaultId: string,
	fileId: string,
	folderId: string | null,
	totalParts: number,
): Promise<VaultWorkerCalls['prepareUpload']['result']> {
	return callVaultWorker('prepareUpload', {
		uploadToken,
		totalParts,
		fileKeyAad: fileKeyAAD(vaultId, fileId, folderId),
	});
}

export function encryptUploadMetadata(
	uploadToken: string,
	metadata: VaultMetadata,
	vaultId: string,
	fileId: string,
): Promise<VaultWorkerCalls['encryptUploadMetadata']['result']> {
	const metadataBytes = encodeJSON(metadata);

	return callVaultWorker(
		'encryptUploadMetadata',
		{
			uploadToken,
			metadataBytes,
			metadataAad: fileMetadataAAD(vaultId, fileId),
		},
		[metadataBytes],
	);
}

export function encryptUploadThumbnail(
	uploadToken: string,
	thumbnailBytes: VaultBytes,
	aad: VaultAAD,
): Promise<VaultWorkerCalls['encryptUploadThumbnail']['result']> {
	const payload = toWorkerBytes(thumbnailBytes);

	return callVaultWorker(
		'encryptUploadThumbnail',
		{ uploadToken, thumbnailBytes: payload, aad: normalizeAAD(aad) },
		[payload],
	);
}

export function encryptUploadPart(
	uploadToken: string,
	chunkBytes: VaultBytes,
	aad?: VaultAAD,
): Promise<VaultWorkerCalls['encryptUploadPart']['result']> {
	const payload = toWorkerBytes(chunkBytes);

	return callVaultWorker(
		'encryptUploadPart',
		{ uploadToken, chunkBytes: payload, aad: normalizeAAD(aad) },
		[payload],
	);
}

export function hashUploadPayload(bytes: VaultBytes): Promise<{ hash: string }> {
	const payload = toWorkerBytes(bytes);

	return callVaultWorker('hashUploadPayload', { bytes: payload }, [payload]);
}

export function finalizeUpload(
	uploadToken: string,
	vaultId: string,
	fileId: string,
	manifest: VaultMetadata,
	partHashes: string[],
): Promise<VaultWorkerCalls['finalizeUpload']['result']> {
	const manifestBytes = encodeJSON(manifest);

	return callVaultWorker(
		'finalizeUpload',
		{
			uploadToken,
			manifestBytes,
			partHashes,
			manifestAad: fileManifestAAD(vaultId, fileId),
		},
		[manifestBytes],
	);
}

export function clearUploadContext(uploadToken: string): Promise<{ cleared: boolean }> {
	return callVaultWorker('clearUploadContext', { uploadToken });
}
