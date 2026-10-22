import { normalizeAAD, toBase64, toWorkerBytes } from './bytes';
import { touchSessionUnlock } from './session';
import type { FileCryptoRecord, VaultAAD, VaultBytes, VaultMetadata, VaultWorkerCalls } from './types';
import { callVaultWorker } from './worker-client';

export function generateFileKey(): Promise<{ fileKey: string }> {
	touchSessionUnlock();
	return callVaultWorker('generateFileKey', {});
}

export function openFileContext(contextId: string, record: FileCryptoRecord): Promise<{ metadata: string; manifest: string }> {
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
	metadata: VaultMetadata,
	masterKey?: VaultBytes | string
): Promise<{ encryptedMetadata: string }> {
	touchSessionUnlock();
	return callVaultWorker('encryptFileMetadata', { metadata, masterKey: toBase64(masterKey), aad: 'arkive:file-metadata:v1' });
}

export function encryptFileMetadataForFile(
	metadata: VaultMetadata,
	vaultId: string,
	fileId: string,
	masterKey?: VaultBytes | string
): Promise<{ encryptedMetadata: string }> {
	touchSessionUnlock();
	return callVaultWorker('encryptFileMetadata', {
		metadata,
		masterKey: toBase64(masterKey),
		aad: `arkive:file-metadata:v1:${vaultId}:${fileId}`
	});
}

export function encryptFileMetadataInContext(
	contextId: string,
	metadata: VaultMetadata,
	vaultId: string,
	fileId: string
): Promise<{ encryptedMetadata: string }> {
	touchSessionUnlock();
	return callVaultWorker('encryptFileMetadataInContext', {
		contextId,
		metadata,
		aad: `arkive:file-metadata:v1:${vaultId}:${fileId}`
	});
}

export function decryptFileMetadata(
	encryptedMetadata: string | VaultBytes,
	masterKey?: VaultBytes | string
): Promise<{ metadata: VaultMetadata }> {
	touchSessionUnlock();
	return callVaultWorker('decryptFileMetadata', {
		encryptedMetadata: toBase64(encryptedMetadata),
		masterKey: toBase64(masterKey),
		aad: 'arkive:file-metadata:v1'
	});
}

export function encryptFolderName(
	metadata: VaultMetadata,
	masterKey?: VaultBytes | string
): Promise<{ encryptedMetadata: string }> {
	touchSessionUnlock();
	return callVaultWorker('encryptFolderMetadata', {
		metadata,
		masterKey: toBase64(masterKey),
		aad: 'arkive:folder-name:v1'
	});
}

export function encryptFolderMetadata(
	metadata: VaultMetadata,
	masterKey?: VaultBytes | string
): Promise<{ encryptedMetadata: string }> {
	touchSessionUnlock();
	return callVaultWorker('encryptFolderMetadata', {
		metadata,
		masterKey: toBase64(masterKey),
		aad: 'arkive:folder-metadata:v1'
	});
}

export function decryptFolderName(
	encryptedMetadata: string | VaultBytes,
	masterKey?: VaultBytes | string
): Promise<{ metadata: VaultMetadata }> {
	touchSessionUnlock();
	return callVaultWorker('decryptFolderMetadata', {
		encryptedMetadata: toBase64(encryptedMetadata),
		masterKey: toBase64(masterKey),
		aad: 'arkive:folder-name:v1'
	});
}

export function decryptFolderMetadata(
	encryptedMetadata: string | VaultBytes,
	masterKey?: VaultBytes | string
): Promise<{ metadata: VaultMetadata }> {
	touchSessionUnlock();
	return callVaultWorker('decryptFolderMetadata', {
		encryptedMetadata: toBase64(encryptedMetadata),
		masterKey: toBase64(masterKey),
		aad: 'arkive:folder-metadata:v1'
	});
}

export function encryptFileKey(
	fileKey: VaultBytes | string,
	masterKey?: VaultBytes | string,
	aad?: VaultAAD
): Promise<{ encryptedFileKey: string }> {
	touchSessionUnlock();
	return callVaultWorker('encryptFileKey', {
		fileKey: toBase64(fileKey),
		masterKey: toBase64(masterKey),
		aad: normalizeAAD(aad) || 'arkive:file-key:v1'
	});
}

export function encryptChunk(
	chunkBytes: VaultBytes | string,
	fileKey: VaultBytes | string,
	aad?: VaultAAD
): Promise<{ encryptedChunk: string }> {
	touchSessionUnlock();
	return callVaultWorker('encryptChunk', {
		chunkBytes: toBase64(chunkBytes),
		fileKey: toBase64(fileKey),
		aad: normalizeAAD(aad)
	});
}

export function decryptChunk(
	encryptedChunk: VaultBytes | string,
	fileKey: VaultBytes | string,
	aad?: VaultAAD
): Promise<VaultWorkerCalls['decryptChunk']['result']> {
	touchSessionUnlock();
	return callVaultWorker('decryptChunk', {
		encryptedChunk: toBase64(encryptedChunk),
		fileKey: toBase64(fileKey),
		aad: normalizeAAD(aad)
	});
}
