import { toWorkerBytes } from './bytes';
import { touchSessionUnlock } from './session';
import type {
	EncryptedSearchFile,
	EncryptedSearchFolder,
	VaultBytes,
	VaultWorkerCalls
} from './types';
import { callVaultWorker } from './worker-client';

function normalizeSearchText(value: unknown): string {
	return String(value || '')
		.toLowerCase()
		.normalize('NFKD')
		.replace(/[^\p{L}\p{N}.]+/gu, ' ')
		.replace(/\s+/g, ' ')
		.trim();
}

function trigramsForText(value: unknown): string[] {
	const normalized = normalizeSearchText(value);
	if (!normalized) return [];
	if (normalized.length < 3) return [normalized];
	const terms: string[] = [];
	for (let i = 0; i <= normalized.length - 3; i += 1) terms.push(normalized.slice(i, i + 3));
	return [...new Set(terms)];
}

export async function createSearchTokens(text: string, vaultId: string, maxTerms = 32): Promise<string[]> {
	touchSessionUnlock();
	const terms = trigramsForText(text).slice(0, maxTerms);
	if (!terms.length) return [];
	const result = await callVaultWorker('createSearchTokens', { vaultId, terms });
	return result.tokens;
}

export function decryptSearchFile(file: EncryptedSearchFile): Promise<VaultWorkerCalls['decryptSearchFile']['result']> {
	touchSessionUnlock();
	return callVaultWorker('decryptSearchFile', {
		fileId: file.id,
		vaultId: file.vaultId,
		encryptedMetadata: file.encryptedMetadata,
		encryptedFileKey: file.encryptedFileKey,
		fileKeyAad: `arkive:file-key:v1:${file.vaultId}:${file.id}`,
		metadataAad: `arkive:file-metadata:v1:${file.vaultId}:${file.id}`
	});
}

export function decryptSearchFolder(folder: EncryptedSearchFolder): Promise<VaultWorkerCalls['decryptSearchFolder']['result']> {
	touchSessionUnlock();
	return callVaultWorker('decryptSearchFolder', {
		encryptedName: folder.encryptedName,
		encryptedMetadata: folder.encryptedMetadata,
		nameAad: 'arkive:folder-name:v1',
		metadataAad: 'arkive:folder-metadata:v1'
	});
}

export function decryptSearchThumbnail(
	file: EncryptedSearchFile,
	encryptedThumbnail: VaultBytes
): Promise<{ thumbnailBytes: Uint8Array }> {
	touchSessionUnlock();
	const payload = toWorkerBytes(encryptedThumbnail);
	return callVaultWorker(
		'decryptSearchThumbnail',
		{
			encryptedFileKey: file.encryptedFileKey,
			encryptedThumbnail: payload,
			fileKeyAad: `arkive:file-key:v1:${file.vaultId}:${file.id}`,
			thumbnailAad: `arkive:file-thumbnail:v1:${file.vaultId}:${file.id}`
		},
		[payload]
	);
}
