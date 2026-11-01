import { decodeJSON, encodeJSON, normalizeAAD, toBase64, toWorkerBytes } from './bytes';
import { touchSessionUnlock } from './session';
import type { FileCryptoRecord, VaultAAD, VaultBytes, VaultMetadata } from './types';
import { callVaultWorker } from './worker-client';

export function openFileContext(contextId: string, record: FileCryptoRecord): Promise<{ metadataBytes: Uint8Array; manifestBytes: Uint8Array }> {
	touchSessionUnlock();
	return callVaultWorker('openFileContext', {
		contextId,
		encryptedFileKey: record.encryptedFileKey,
		encryptedMetadata: record.encryptedMetadata,
		encryptedManifest: record.encryptedManifest,
		fileKeyAad: `arkive:file-key:v1:${record.vaultId}:${record.fileId}`,
		metadataAad: `arkive:file-metadata:v1:${record.vaultId}:${record.fileId}`,
		manifestAad: `arkive:file-manifest:v1:${record.vaultId}:${record.fileId}`
	});
}

export function closeFileContext(contextId: string): Promise<{ cleared: boolean }> {
	return callVaultWorker('closeFileContext', { contextId });
}

export function decryptFileChunk(
	contextId: string,
	encryptedChunk: VaultBytes,
	aad: VaultAAD,
	expectedHash = ''
): Promise<{ chunkBytes: Uint8Array }> {
	touchSessionUnlock();
	const payload = toWorkerBytes(encryptedChunk);
	return callVaultWorker('decryptFileChunk', { contextId, encryptedChunk: payload, aad: normalizeAAD(aad), expectedHash }, [payload]);
}

export function encryptFileMetadata(
	metadata: VaultMetadata
): Promise<{ encryptedMetadata: string }> {
	touchSessionUnlock();
	const metadataBytes = encodeJSON(metadata);
	return callVaultWorker('encryptFileMetadata', { metadataBytes, aad: 'arkive:file-metadata:v1' }, [metadataBytes]);
}

export function encryptFileMetadataForFile(
	metadata: VaultMetadata,
	vaultId: string,
	fileId: string
): Promise<{ encryptedMetadata: string }> {
	touchSessionUnlock();
	const metadataBytes = encodeJSON(metadata);
	return callVaultWorker('encryptFileMetadata', {
		metadataBytes,
		aad: `arkive:file-metadata:v1:${vaultId}:${fileId}`
	}, [metadataBytes]);
}

export function encryptFileMetadataInContext(
	contextId: string,
	metadata: VaultMetadata,
	vaultId: string,
	fileId: string
): Promise<{ encryptedMetadata: string }> {
	touchSessionUnlock();
	const metadataBytes = encodeJSON(metadata);
	return callVaultWorker('encryptFileMetadataInContext', {
		contextId,
		metadataBytes,
		aad: `arkive:file-metadata:v1:${vaultId}:${fileId}`
	}, [metadataBytes]);
}

export async function decryptFileMetadata(
	encryptedMetadata: string | VaultBytes
): Promise<{ metadata: VaultMetadata }> {
	touchSessionUnlock();
	const result = await callVaultWorker('decryptFileMetadata', {
		encryptedMetadata: toBase64(encryptedMetadata),
		aad: 'arkive:file-metadata:v1'
	});
	return { metadata: decodeJSON<VaultMetadata>(result.metadataBytes) };
}

export function encryptFolderName(
	metadata: VaultMetadata
): Promise<{ encryptedMetadata: string }> {
	touchSessionUnlock();
	const metadataBytes = encodeJSON(metadata);
	return callVaultWorker('encryptFolderMetadata', {
		metadataBytes,
		aad: 'arkive:folder-name:v1'
	}, [metadataBytes]);
}

export function encryptFolderMetadata(
	metadata: VaultMetadata
): Promise<{ encryptedMetadata: string }> {
	touchSessionUnlock();
	const metadataBytes = encodeJSON(metadata);
	return callVaultWorker('encryptFolderMetadata', {
		metadataBytes,
		aad: 'arkive:folder-metadata:v1'
	}, [metadataBytes]);
}
export async function decryptFolderName(
	encryptedMetadata: string | VaultBytes
): Promise<{ metadata: VaultMetadata }> {
	touchSessionUnlock();
	const result = await callVaultWorker('decryptFolderMetadata', {
		encryptedMetadata: toBase64(encryptedMetadata),
		aad: 'arkive:folder-name:v1'
	});
	return { metadata: decodeJSON<VaultMetadata>(result.metadataBytes) };
}

export async function decryptFolderMetadata(
	encryptedMetadata: string | VaultBytes
): Promise<{ metadata: VaultMetadata }> {
	touchSessionUnlock();
	const result = await callVaultWorker('decryptFolderMetadata', {
		encryptedMetadata: toBase64(encryptedMetadata),
		aad: 'arkive:folder-metadata:v1'
	});
	return { metadata: decodeJSON<VaultMetadata>(result.metadataBytes) };
}
