import { normalizeAAD, toWorkerBytes } from './bytes';
import { touchSessionUnlock } from './session';
import type { VaultAAD, VaultBytes, VaultMetadata, VaultWorkerCalls } from './types';
import { callVaultWorker } from './worker-client';

export function prepareUpload(
	uploadToken: string,
	vaultId: string,
	fileId: string,
	metadata: VaultMetadata,
	totalParts: number
): Promise<VaultWorkerCalls['prepareUpload']['result']> {
	touchSessionUnlock();
	return callVaultWorker('prepareUpload', {
		uploadToken,
		vaultId,
		fileId,
		metadata,
		totalParts,
		metadataAad: `arkive:file-metadata:v1:${vaultId}:${fileId}`,
		fileKeyAad: `arkive:file-key:v1:${vaultId}:${fileId}`
	});
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
	return callVaultWorker('finalizeUpload', {
		uploadToken,
		manifest,
		partHashes,
		manifestAad: `arkive:file-manifest:v1:${vaultId}:${fileId}`
	});
}

export function clearUploadContext(uploadToken: string): Promise<{ cleared: boolean }> {
	return callVaultWorker('clearUploadContext', { uploadToken });
}
