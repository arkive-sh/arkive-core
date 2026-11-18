import assert from 'node:assert/strict';
import { after, before, test } from 'node:test';
import { createServer } from 'vite';

if (!Uint8Array.prototype.toBase64) {
	Object.defineProperty(Uint8Array.prototype, 'toBase64', {
		value() {
			return Buffer.from(this).toString('base64');
		},
	});
}

let vite;
let uploadOperations;
let fileOperations;
let vaultOperations;
let workerState;
let lockVault;

before(async () => {
	vite = await createServer({
		configFile: false,
		root: process.cwd(),
		logLevel: 'silent',
		server: { middlewareMode: true, hmr: false },
		appType: 'custom',
	});
	({ upload_operations: uploadOperations } = await vite.ssrLoadModule(
		'/src/lib/vault/crypto-worker/upload-operations.ts',
	));
	({ file_operations: fileOperations } = await vite.ssrLoadModule(
		'/src/lib/vault/crypto-worker/file-operations.ts',
	));
	({ vault_operations: vaultOperations } = await vite.ssrLoadModule(
		'/src/lib/vault/crypto-worker/vault-operations.ts',
	));
	({ workerState, lockVault } = await vite.ssrLoadModule(
		'/src/lib/vault/crypto-worker/runtime.ts',
	));
});

after(async () => {
	lockVault?.();
	await vite?.close();
});

test('version upload reuses the file key and keeps both versions decryptable', async () => {
	const aad = await vite.ssrLoadModule('/src/lib/vault/aad.ts');
	const exportKey = Uint8Array.from({ length: 32 }, (_, index) => index + 1);
	const vault = await vaultOperations.createInitialVault({ exportKey: exportKey.slice().buffer });
	await vaultOperations.unlockVault({
		exportKey: exportKey.slice().buffer,
		salt: vault.vaultSalt,
		encryptedMasterKey: vault.encryptedMasterKey,
		aad: aad.VAULT_MASTER_KEY_AAD,
	});
	vault.masterKey.fill(0);
	exportKey.fill(0);

	const vaultId = 'version-test-vault';
	const fileId = 'version-test-file';
	const folderId = 'version-test-folder';
	const fileKeyAad = aad.fileKeyAAD(vaultId, fileId, folderId);
	const metadataAad = aad.fileMetadataAAD(vaultId, fileId);
	const manifestAad = aad.fileManifestAAD(vaultId, fileId);
	const chunkSize = 1024;
	const chunkAad = aad.fileChunkAAD(vaultId, fileId, 1, chunkSize, 1);
	const versions = [];

	try {
		for (const [index, content] of ['first version content', 'second version content'].entries()) {
			const uploadToken = `version-upload-${index + 1}`;
			const prepared = await uploadOperations.prepareUpload({
				uploadToken,
				totalParts: 1,
				fileKeyAad,
				...(versions[0] ? { encryptedFileKey: versions[0].encryptedFileKey } : {}),
			});
			if (versions[0]) {
				assert.equal(
					prepared.encryptedFileKey,
					versions[0].encryptedFileKey,
					'version upload must keep the existing wrapped file key',
				);
			}

			const metadataBytes = new TextEncoder().encode(
				JSON.stringify({ name: 'notes.txt', version: index + 1 }),
			);
			const encryptedMetadata = await uploadOperations.encryptUploadMetadata({
				uploadToken,
				metadataBytes: metadataBytes.slice().buffer,
				metadataAad,
			});
			const chunkBytes = new TextEncoder().encode(content);
			const encryptedChunk = await uploadOperations.encryptUploadPart({
				uploadToken,
				chunkBytes: chunkBytes.slice(),
				aad: chunkAad,
			});
			const manifestBytes = new TextEncoder().encode(
				JSON.stringify({ fileId, version: index + 1 }),
			);
			const finalized = await uploadOperations.finalizeUpload({
				uploadToken,
				manifestBytes: manifestBytes.slice().buffer,
				partHashes: [encryptedChunk.encryptedHash],
				manifestAad,
			});
			versions.push({
				encryptedFileKey: prepared.encryptedFileKey,
				encryptedMetadata: encryptedMetadata.encryptedMetadata,
				encryptedManifest: finalized.encryptedManifest,
				encryptedChunk: encryptedChunk.encryptedChunk,
				encryptedHash: encryptedChunk.encryptedHash,
				metadataBytes,
				manifestBytes,
				chunkBytes,
			});
		}

		for (const [index, version] of versions.entries()) {
			const contextId = `version-reader-${index + 1}`;
			const opened = await fileOperations.openFileContext({
				contextId,
				encryptedFileKey: version.encryptedFileKey,
				encryptedMetadata: version.encryptedMetadata,
				encryptedManifest: version.encryptedManifest,
				fileKeyAad,
				metadataAad,
				manifestAad,
			});
			assert.deepEqual(opened.metadataBytes, version.metadataBytes);
			assert.deepEqual(opened.manifestBytes, version.manifestBytes);
			const decryptedChunk = await fileOperations.decryptFileChunk({
				contextId,
				encryptedChunk: version.encryptedChunk.slice(),
				aad: chunkAad,
				expectedHash: version.encryptedHash,
			});
			assert.deepEqual(decryptedChunk.chunkBytes, version.chunkBytes);
			await fileOperations.closeFileContext({ contextId });
		}
	} finally {
		for (const version of versions) {
			version.metadataBytes.fill(0);
			version.manifestBytes.fill(0);
			version.chunkBytes.fill(0);
			version.encryptedChunk.fill(0);
		}
		for (const uploadToken of ['version-upload-1', 'version-upload-2'])
			await uploadOperations.clearUploadContext({ uploadToken });
		for (const contextId of ['version-reader-1', 'version-reader-2'])
			await fileOperations.closeFileContext({ contextId });
		if (workerState.unlockedMasterKey) await vaultOperations.lockVault({});
	}
});
