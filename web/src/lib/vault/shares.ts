import { normalizeAAD, toWorkerBytes } from './bytes';
import { fileKeyAAD, fileManifestAAD, fileMetadataAAD, shareFileKeyAAD, shareKeyAAD } from './aad';
import type {
	PublicShareCryptoRecord,
	ShareFileRecord,
	VaultAAD,
	VaultBytes,
	VaultWorkerCalls,
} from './types';
import { callVaultWorker } from './worker-client';

export function prepareShare(
	record: ShareFileRecord,
	token: string,
): Promise<VaultWorkerCalls['prepareShare']['result']> {
	return callVaultWorker('prepareShare', {
		vaultId: record.vaultId,
		fileId: record.fileId,
		token,
		encryptedFileKey: record.encryptedFileKey,
		fileKeyAad: fileKeyAAD(record.vaultId, record.fileId, record.folderId),
		shareKeyAad: shareKeyAAD(token),
		shareFileKeyAad: shareFileKeyAAD(record.fileId, token),
	});
}

export function openShareKey(
	encryptedShareKey: string,
	token: string,
): Promise<{ shareSecret: Uint8Array<ArrayBuffer> }> {
	return callVaultWorker('openShareKey', {
		encryptedShareKey,
		shareKeyAad: shareKeyAAD(token),
	});
}

export function openPublicShareContext(
	contextId: string,
	record: PublicShareCryptoRecord,
	shareSecret: Uint8Array<ArrayBuffer>,
): Promise<{ metadataBytes: Uint8Array; manifestBytes: Uint8Array }> {
	const secret = toWorkerBytes(shareSecret);
	shareSecret.fill(0);

	return callVaultWorker(
		'openPublicShareContext',
		{
			contextId,
			shareSecret: secret,
			encryptedFileKeyForShare: record.encryptedFileKeyForShare,
			encryptedMetadata: record.encryptedMetadata,
			encryptedManifest: record.encryptedManifest,
			shareFileKeyAad: shareFileKeyAAD(record.fileId, record.token),
			metadataAad: fileMetadataAAD(record.vaultId, record.fileId),
			manifestAad: fileManifestAAD(record.vaultId, record.fileId),
		},
		[secret],
	);
}

export function closePublicShareContext(contextId: string): Promise<{ cleared: boolean }> {
	return callVaultWorker('closePublicShareContext', { contextId });
}

export function decryptPublicShareChunk(
	contextId: string,
	encryptedChunk: VaultBytes,
	aad: VaultAAD,
	expectedHash = '',
	hashEncoding: 'base64' | 'hex' = 'base64',
): Promise<{ chunkBytes: Uint8Array }> {
	const payload = toWorkerBytes(encryptedChunk);

	return callVaultWorker(
		'decryptPublicShareChunk',
		{ contextId, encryptedChunk: payload, aad: normalizeAAD(aad), expectedHash, hashEncoding },
		[payload],
	);
}
