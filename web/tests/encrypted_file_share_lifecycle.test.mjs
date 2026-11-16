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
let shareOperations;
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
	({ share_operations: shareOperations } = await vite.ssrLoadModule(
		'/src/lib/vault/crypto-worker/share-operations.ts',
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

test('worker crypto preserves files across upload, lock, rename, move, and public sharing', async () => {
	const aad = await vite.ssrLoadModule('/src/lib/vault/aad.ts');
	const exportKey = Uint8Array.from({ length: 32 }, (_, index) => index);
	const vault = await vaultOperations.createInitialVault({ exportKey: exportKey.slice().buffer });
	await vaultOperations.unlockVault({
		exportKey: exportKey.slice().buffer,
		salt: vault.vaultSalt,
		encryptedMasterKey: vault.encryptedMasterKey,
		aad: aad.VAULT_MASTER_KEY_AAD,
	});
	const uploadToken = 'crypto-test-upload';
	const fileKeyAad = 'file:test:key:v1';
	const metadataAad = 'file:test:metadata:v1';
	const manifestAad = 'file:test:manifest:v1';
	const chunkAad = 'file:test:chunk:0:v1';
	const metadata = new TextEncoder().encode('{"name":"notes.txt"}');
	const manifest = new TextEncoder().encode('{"parts":1}');
	const plaintextChunk = new TextEncoder().encode('encrypted file contents');

	const prepared = await uploadOperations.prepareUpload({ uploadToken, totalParts: 1, fileKeyAad });
	const encryptedMetadata = await uploadOperations.encryptUploadMetadata({
		uploadToken,
		metadataBytes: metadata.slice().buffer,
		metadataAad,
	});
	const encryptedPart = await uploadOperations.encryptUploadPart({
		uploadToken,
		chunkBytes: plaintextChunk.slice(),
		aad: chunkAad,
	});
	const finalized = await uploadOperations.finalizeUpload({
		uploadToken,
		manifestBytes: manifest.slice().buffer,
		partHashes: [encryptedPart.encryptedHash],
		manifestAad,
	});
	const masterKeyBeforeLock = workerState.unlockedMasterKey;
	await vaultOperations.lockVault({});
	assert.equal(workerState.unlockedMasterKey, null);
	assert.ok(masterKeyBeforeLock.every((byte) => byte === 0), 'locking should clear the master key bytes');
	await vaultOperations.unlockVault({
		exportKey: exportKey.slice().buffer,
		salt: vault.vaultSalt,
		encryptedMasterKey: vault.encryptedMasterKey,
		aad: aad.VAULT_MASTER_KEY_AAD,
	});
	const opened = await fileOperations.openFileContext({
		contextId: 'crypto-test-reader',
		encryptedFileKey: prepared.encryptedFileKey,
		encryptedMetadata: encryptedMetadata.encryptedMetadata,
		encryptedManifest: finalized.encryptedManifest,
		fileKeyAad,
		metadataAad,
		manifestAad,
	});
	assert.deepEqual(opened.metadataBytes, metadata);
	assert.deepEqual(opened.manifestBytes, manifest);

	const decrypted = await fileOperations.decryptFileChunk({
		contextId: 'crypto-test-reader',
		encryptedChunk: encryptedPart.encryptedChunk.slice(),
		aad: chunkAad,
		expectedHash: encryptedPart.encryptedHash,
	});
	assert.deepEqual(decrypted.chunkBytes, plaintextChunk);
	await fileOperations.closeFileContext({ contextId: 'crypto-test-reader' });

	const movedFileKeyAad = 'file:test:key:parent-folder:v1';
	const movedFileKey = await fileOperations.rewrapFileKey({
		encryptedFileKey: prepared.encryptedFileKey,
		oldAad: fileKeyAad,
		newAad: movedFileKeyAad,
	});
	const renamedMetadata = new TextEncoder().encode('{"name":"renamed.txt"}');
	const encryptedRenamedMetadata = await fileOperations.encryptFileMetadataForFile({
		metadataBytes: renamedMetadata.slice().buffer,
		encryptedFileKey: movedFileKey.encryptedFileKey,
		fileKeyAad: movedFileKeyAad,
		metadataAad,
	});
	await assert.rejects(
		fileOperations.openFileContext({
			contextId: 'wrong-parent-reader',
			encryptedFileKey: movedFileKey.encryptedFileKey,
			encryptedMetadata: encryptedRenamedMetadata.encryptedMetadata,
			encryptedManifest: finalized.encryptedManifest,
			fileKeyAad,
			metadataAad,
			manifestAad,
		}),
	);
	const movedFile = await fileOperations.openFileContext({
		contextId: 'moved-reader',
		encryptedFileKey: movedFileKey.encryptedFileKey,
		encryptedMetadata: encryptedRenamedMetadata.encryptedMetadata,
		encryptedManifest: finalized.encryptedManifest,
		fileKeyAad: movedFileKeyAad,
		metadataAad,
		manifestAad,
	});
	assert.deepEqual(movedFile.metadataBytes, renamedMetadata);
	const movedChunk = await fileOperations.decryptFileChunk({
		contextId: 'moved-reader',
		encryptedChunk: encryptedPart.encryptedChunk.slice(),
		aad: chunkAad,
		expectedHash: encryptedPart.encryptedHash,
	});
	assert.deepEqual(movedChunk.chunkBytes, plaintextChunk);
	await assert.rejects(
		fileOperations.decryptFileChunk({
			contextId: 'moved-reader',
			encryptedChunk: encryptedPart.encryptedChunk.slice(),
			aad: 'wrong-file-context',
			expectedHash: encryptedPart.encryptedHash,
		}),
	);
	await assert.rejects(
		fileOperations.decryptFileChunk({
			contextId: 'moved-reader',
			encryptedChunk: encryptedPart.encryptedChunk.slice(),
			aad: chunkAad,
			expectedHash: 'wrong-hash',
		}),
		/Encrypted chunk hash mismatch/,
	);

	const shareToken = 'test-public-token';
	const share = await shareOperations.prepareShare({
		vaultId: 'test-vault',
		fileId: 'test-file',
		token: shareToken,
		encryptedFileKey: movedFileKey.encryptedFileKey,
		fileKeyAad: movedFileKeyAad,
		shareKeyAad: aad.shareKeyAAD(shareToken),
		shareFileKeyAad: aad.shareFileKeyAAD('test-file', shareToken),
	});
	const ownerShareKey = await shareOperations.openShareKey({
		encryptedShareKey: share.encryptedShareKey,
		shareKeyAad: aad.shareKeyAAD(shareToken),
	});
	assert.deepEqual(ownerShareKey.shareSecret, share.shareSecret);

	const tamperedChunk = encryptedPart.encryptedChunk.slice();
	tamperedChunk[tamperedChunk.length - 1] ^= 1;
	await assert.rejects(
		fileOperations.decryptFileChunk({
			contextId: 'moved-reader',
			encryptedChunk: tamperedChunk,
			aad: chunkAad,
			expectedHash: '',
		}),
	);

	await fileOperations.closeFileContext({ contextId: 'moved-reader' });
	await vaultOperations.lockVault({});
	const publicFile = await shareOperations.openPublicShareContext({
		contextId: 'public-reader',
		shareSecret: share.shareSecret.slice(),
		encryptedFileKeyForShare: share.encryptedFileKeyForShare,
		encryptedMetadata: encryptedRenamedMetadata.encryptedMetadata,
		encryptedManifest: finalized.encryptedManifest,
		shareFileKeyAad: aad.shareFileKeyAAD('test-file', shareToken),
		metadataAad,
		manifestAad,
	});
	assert.deepEqual(publicFile.metadataBytes, renamedMetadata);
	assert.deepEqual(publicFile.manifestBytes, manifest);
	const publicChunk = await shareOperations.decryptPublicShareChunk({
		contextId: 'public-reader',
		encryptedChunk: encryptedPart.encryptedChunk.slice(),
		aad: chunkAad,
		expectedHash: encryptedPart.encryptedHash,
		hashEncoding: 'base64',
	});
	assert.deepEqual(publicChunk.chunkBytes, plaintextChunk);
	await assert.rejects(
		shareOperations.openPublicShareContext({
			contextId: 'wrong-token-reader',
			shareSecret: share.shareSecret.slice(),
			encryptedFileKeyForShare: share.encryptedFileKeyForShare,
			encryptedMetadata: encryptedRenamedMetadata.encryptedMetadata,
			encryptedManifest: finalized.encryptedManifest,
			shareFileKeyAad: aad.shareFileKeyAAD('test-file', 'wrong-token'),
			metadataAad,
			manifestAad,
		}),
	);
	await shareOperations.closePublicShareContext({ contextId: 'public-reader' });

	await assert.rejects(
		fileOperations.decryptFileChunk({
			contextId: 'moved-reader',
			encryptedChunk: encryptedPart.encryptedChunk.slice(),
			aad: chunkAad,
			expectedHash: encryptedPart.encryptedHash,
		}),
		/File context is missing/,
	);
	lockVault();
});
