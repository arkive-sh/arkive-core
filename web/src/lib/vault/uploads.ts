import { encodeJSON, normalizeAAD, toWorkerBytes } from './bytes';
import { touchSessionUnlock } from './session';
import type { VaultAAD, VaultBytes, VaultMetadata, VaultWorkerCalls } from './types';
import { callVaultWorker } from './worker-client';

export function prepareUpload(
	uploadToken: string,
	vaultId: string,
	fileId: string,
	totalParts: number
): Promise<VaultWorkerCalls['prepareUpload']['result']> {
	touchSessionUnlock();
	return callVaultWorker('prepareUpload', {
		uploadToken,
		totalParts,
		fileKeyAad: `arkive:file-key:v1:${vaultId}:${fileId}`
	});
}

export function encryptUploadMetadata(
	uploadToken: string,
	metadata: VaultMetadata,
	vaultId: string,
	fileId: string
): Promise<VaultWorkerCalls['encryptUploadMetadata']['result']> {
	touchSessionUnlock();
	const metadataBytes = encodeJSON(metadata);
	return callVaultWorker('encryptUploadMetadata', {
		uploadToken,
		metadataBytes,
		metadataAad: `arkive:file-metadata:v1:${vaultId}:${fileId}`
	}, [metadataBytes]);
}

export function encryptUploadThumbnail(
	uploadToken: string,
	thumbnailBytes: VaultBytes,
	aad: VaultAAD
): Promise<VaultWorkerCalls['encryptUploadThumbnail']['result']> {
	touchSessionUnlock();
	const payload = toWorkerBytes(thumbnailBytes);
	return callVaultWorker('encryptUploadThumbnail', { uploadToken, thumbnailBytes: payload, aad: normalizeAAD(aad) }, [payload]);
}

export function encryptUploadPart(
	uploadToken: string,
	chunkBytes: VaultBytes,
	aad?: VaultAAD
): Promise<VaultWorkerCalls['encryptUploadPart']['result']> {
	touchSessionUnlock();
	const payload = toWorkerBytes(chunkBytes);
	return callVaultWorker('encryptUploadPart', { uploadToken, chunkBytes: payload, aad: normalizeAAD(aad) }, [payload]);
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
	partHashes: string[]
): Promise<VaultWorkerCalls['finalizeUpload']['result']> {
	touchSessionUnlock();
	const manifestBytes = encodeJSON(manifest);
	return callVaultWorker('finalizeUpload', {
		uploadToken,
		manifestBytes,
		partHashes,
		manifestAad: `arkive:file-manifest:v1:${vaultId}:${fileId}`
	}, [manifestBytes]);
}

export function clearUploadContext(uploadToken: string): Promise<{ cleared: boolean }> {
	return callVaultWorker('clearUploadContext', { uploadToken });
}
