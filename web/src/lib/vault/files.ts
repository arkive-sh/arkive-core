import { decodeJSON, encodeJSON, normalizeAAD, toBase64, toWorkerBytes } from './bytes';
import {
	FILE_METADATA_AAD,
	fileKeyAAD,
	fileManifestAAD,
	fileMetadataAAD,
	folderMetadataAAD,
	folderNameAAD,
} from './aad';
import type {
	FileCryptoRecord,
	ShareFileRecord,
	VaultAAD,
	VaultBytes,
	VaultMetadata,
} from './types';
import { callVaultWorker } from './worker-client';

export function openFileContext(
	contextId: string,
	record: FileCryptoRecord,
): Promise<{ metadataBytes: Uint8Array; manifestBytes: Uint8Array }> {
	return callVaultWorker('openFileContext', {
		contextId,
		encryptedFileKey: record.encryptedFileKey,
		encryptedMetadata: record.encryptedMetadata,
		encryptedManifest: record.encryptedManifest,
		fileKeyAad: fileKeyAAD(record.vaultId, record.fileId, record.folderId),
		metadataAad: fileMetadataAAD(record.vaultId, record.fileId),
		manifestAad: fileManifestAAD(record.vaultId, record.fileId),
	});
}

export function closeFileContext(contextId: string): Promise<{ cleared: boolean }> {
	return callVaultWorker('closeFileContext', { contextId });
}

export function decryptFileChunk(
	contextId: string,
	encryptedChunk: VaultBytes,
	aad: VaultAAD,
	expectedHash = '',
): Promise<{ chunkBytes: Uint8Array }> {
	const payload = toWorkerBytes(encryptedChunk);

	return callVaultWorker(
		'decryptFileChunk',
		{ contextId, encryptedChunk: payload, aad: normalizeAAD(aad), expectedHash },
		[payload],
	);
}

export function encryptFileMetadata(
	metadata: VaultMetadata,
): Promise<{ encryptedMetadata: string }> {
	const metadataBytes = encodeJSON(metadata);

	return callVaultWorker('encryptFileMetadata', { metadataBytes, aad: FILE_METADATA_AAD }, [
		metadataBytes,
	]);
}

export function encryptFileMetadataForFile(
	metadata: VaultMetadata,
	vaultId: string,
	fileId: string,
): Promise<{ encryptedMetadata: string }> {
	const metadataBytes = encodeJSON(metadata);

	return callVaultWorker(
		'encryptFileMetadata',
		{
			metadataBytes,
			aad: fileMetadataAAD(vaultId, fileId),
		},
		[metadataBytes],
	);
}

export function encryptFileMetadataInContext(
	contextId: string,
	metadata: VaultMetadata,
	vaultId: string,
	fileId: string,
): Promise<{ encryptedMetadata: string }> {
	const metadataBytes = encodeJSON(metadata);

	return callVaultWorker(
		'encryptFileMetadataInContext',
		{
			contextId,
			metadataBytes,
			aad: fileMetadataAAD(vaultId, fileId),
		},
		[metadataBytes],
	);
}

export async function decryptFileMetadata(
	encryptedMetadata: string | VaultBytes,
): Promise<{ metadata: VaultMetadata }> {
	const result = await callVaultWorker('decryptFileMetadata', {
		encryptedMetadata: toBase64(encryptedMetadata),
		aad: FILE_METADATA_AAD,
	});

	return { metadata: decodeJSON<VaultMetadata>(result.metadataBytes) };
}

export function encryptFolderName(
	metadata: VaultMetadata,
	vaultId: string,
	folderId: string,
	parentFolderId: string | null,
): Promise<{ encryptedMetadata: string }> {
	const metadataBytes = encodeJSON(metadata);

	return callVaultWorker(
		'encryptFolderMetadata',
		{
			metadataBytes,
			aad: folderNameAAD(vaultId, folderId, parentFolderId),
		},
		[metadataBytes],
	);
}

export function encryptFolderMetadata(
	metadata: VaultMetadata,
	vaultId: string,
	folderId: string,
	parentFolderId: string | null,
): Promise<{ encryptedMetadata: string }> {
	const metadataBytes = encodeJSON(metadata);

	return callVaultWorker(
		'encryptFolderMetadata',
		{
			metadataBytes,
			aad: folderMetadataAAD(vaultId, folderId, parentFolderId),
		},
		[metadataBytes],
	);
}
export async function decryptFolderName(
	encryptedMetadata: string | VaultBytes,
	vaultId: string,
	folderId: string,
	parentFolderId: string | null,
): Promise<{ metadata: VaultMetadata }> {
	const result = await callVaultWorker('decryptFolderMetadata', {
		encryptedMetadata: toBase64(encryptedMetadata),
		aad: folderNameAAD(vaultId, folderId, parentFolderId),
	});

	return { metadata: decodeJSON<VaultMetadata>(result.metadataBytes) };
}

export async function decryptFolderMetadata(
	encryptedMetadata: string | VaultBytes,
	vaultId: string,
	folderId: string,
	parentFolderId: string | null,
): Promise<{ metadata: VaultMetadata }> {
	const result = await callVaultWorker('decryptFolderMetadata', {
		encryptedMetadata: toBase64(encryptedMetadata),
		aad: folderMetadataAAD(vaultId, folderId, parentFolderId),
	});

	return { metadata: decodeJSON<VaultMetadata>(result.metadataBytes) };
}

export function rewrapFileKeyForParent(
	record: ShareFileRecord,
	parentFolderId: string | null,
): Promise<{ encryptedFileKey: string }> {
	return callVaultWorker('rewrapFileKey', {
		encryptedFileKey: record.encryptedFileKey,
		oldAad: fileKeyAAD(record.vaultId, record.fileId, record.folderId),
		newAad: fileKeyAAD(record.vaultId, record.fileId, parentFolderId),
	});
}

export function rewrapFolderMetadataForParent(
	record: {
		vaultId: string;
		folderId: string;
		parentFolderId: string | null;
		encryptedName: string;
		encryptedMetadata: string;
	},
	parentFolderId: string | null,
): Promise<{ encryptedName: string; encryptedMetadata: string }> {
	return callVaultWorker('rewrapFolderMetadata', {
		encryptedName: record.encryptedName,
		encryptedMetadata: record.encryptedMetadata,
		oldNameAad: folderNameAAD(record.vaultId, record.folderId, record.parentFolderId),
		oldMetadataAad: folderMetadataAAD(record.vaultId, record.folderId, record.parentFolderId),
		newNameAad: folderNameAAD(record.vaultId, record.folderId, parentFolderId),
		newMetadataAad: folderMetadataAAD(record.vaultId, record.folderId, parentFolderId),
	});
}
