import { decodeJSON, toWorkerBytes } from './bytes';
import type {
	EncryptedSearchFile,
	EncryptedSearchFolder,
	MediaDimensions,
	SearchPreview,
	VaultBytes,
	VaultMetadata,
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
	const terms = trigramsForText(text).slice(0, maxTerms);
	if (!terms.length) return [];
	const result = await callVaultWorker('createSearchTokens', { vaultId, terms });
	return result.tokens;
}

export async function decryptSearchFile(file: EncryptedSearchFile): Promise<{ metadata: VaultMetadata & { name?: string; mime?: string; preview?: SearchPreview; dimensions?: MediaDimensions } }> {
	const result = await callVaultWorker('decryptSearchFile', {
		fileId: file.id,
		vaultId: file.vaultId,
		encryptedMetadata: file.encryptedMetadata,
		encryptedFileKey: file.encryptedFileKey,
		fileKeyAad: `arkive:file-key:v1:${file.vaultId}:${file.id}`,
		metadataAad: `arkive:file-metadata:v1:${file.vaultId}:${file.id}`
	});
	return { metadata: decodeJSON<VaultMetadata & { name?: string; mime?: string; preview?: SearchPreview; dimensions?: MediaDimensions }>(result.metadataBytes) };
}

export async function decryptSearchFolder(folder: EncryptedSearchFolder): Promise<{ name: VaultMetadata & { name?: string }; metadata: VaultMetadata & { name?: string } }> {
	const result = await callVaultWorker('decryptSearchFolder', {
		encryptedName: folder.encryptedName,
		encryptedMetadata: folder.encryptedMetadata,
		nameAad: 'arkive:folder-name:v1',
		metadataAad: 'arkive:folder-metadata:v1'
	});
	let nameBytes: Uint8Array | null = result.nameBytes;
	let metadataBytes: Uint8Array | null = result.metadataBytes;
	try {
		if (!nameBytes || !metadataBytes) throw new Error('The decrypted folder data is incomplete.');
		const name = decodeJSON<VaultMetadata & { name?: string }>(nameBytes);
		nameBytes = null;
		const metadata = decodeJSON<VaultMetadata & { name?: string }>(metadataBytes);
		metadataBytes = null;
		return { name, metadata };
	} finally {
		nameBytes?.fill(0);
		metadataBytes?.fill(0);
		nameBytes = null;
		metadataBytes = null;
	}
}

export function decryptSearchThumbnail(
	file: EncryptedSearchFile,
	encryptedThumbnail: VaultBytes
): Promise<{ thumbnailBytes: Uint8Array }> {
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
