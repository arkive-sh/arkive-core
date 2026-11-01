import { normalizeAAD, toWorkerBytes } from './bytes';
import { touchSessionUnlock } from './session';
import type { PublicShareCryptoRecord, ShareFileRecord, VaultAAD, VaultBytes, VaultWorkerCalls } from './types';
import { callVaultWorker } from './worker-client';

export function prepareShare(
	record: ShareFileRecord,
	token: string
): Promise<VaultWorkerCalls['prepareShare']['result']> {
	touchSessionUnlock();
	return callVaultWorker('prepareShare', {
		vaultId: record.vaultId,
		fileId: record.fileId,
		token,
		encryptedFileKey: record.encryptedFileKey,
		fileKeyAad: `arkive:file-key:v1:${record.vaultId}:${record.fileId}`,
		shareKeyAad: `arkive:share-key:v1:${token}`,
		shareFileKeyAad: `arkive:share-file-key:v1:${record.fileId}:${token}`
	});
}

export function openShareKey(encryptedShareKey: string, token: string): Promise<{ shareSecret: Uint8Array<ArrayBuffer> }> {
	touchSessionUnlock();
	return callVaultWorker('openShareKey', {
		encryptedShareKey,
		shareKeyAad: `arkive:share-key:v1:${token}`
	});
}

export function openPublicShareContext(
	contextId: string,
	record: PublicShareCryptoRecord,
	shareSecret: Uint8Array<ArrayBuffer>
): Promise<{ metadataBytes: Uint8Array; manifestBytes: Uint8Array }> {
	const secret = toWorkerBytes(shareSecret);
	shareSecret.fill(0);
	return callVaultWorker('openPublicShareContext', { contextId, shareSecret: secret, ...record }, [secret]);
}

export function closePublicShareContext(contextId: string): Promise<{ cleared: boolean }> {
	return callVaultWorker('closePublicShareContext', { contextId });
}

export function decryptPublicShareChunk(
	contextId: string,
	encryptedChunk: VaultBytes,
	aad: VaultAAD,
	expectedHash = '',
	hashEncoding: 'base64' | 'hex' = 'base64'
): Promise<{ chunkBytes: Uint8Array }> {
	const payload = toWorkerBytes(encryptedChunk);
	return callVaultWorker(
		'decryptPublicShareChunk',
		{ contextId, encryptedChunk: payload, aad: normalizeAAD(aad), expectedHash, hashEncoding },
		[payload]
	);
}
