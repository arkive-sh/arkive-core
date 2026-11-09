import { deriveRecoveryProofPublicKey, signRecoveryProof } from '../recovery-proof';
import { fromBase64, toBase64, toBase64Url } from './bytes';
import { DecryptionService } from './decryption-service';
import { EncryptionService } from './encryption-service';
import { HashService } from './hash-service';
import { formatRecoveryKey, parseRecoveryKey } from './recovery-key';
import { ENCRYPTION_VERSION } from './crypto-format';
import {
	VAULT_MASTER_KEY_AAD,
	fileKeyAAD,
	fileMetadataAAD,
	folderMetadataAAD,
	folderNameAAD,
	recoveryMasterKeyAAD,
	shareFileKeyAAD,
	shareFolderMetadataAAD,
	shareFolderNameAAD,
	shareKeyAAD,
} from './aad';
import type { VaultWorkerCalls, VaultWorkerMethod } from './types';

type WorkerInput<K extends VaultWorkerMethod> = VaultWorkerCalls[K]['params'];
type WorkerRequest = {
	[K in VaultWorkerMethod]: { id: number; method: K; params: WorkerInput<K> };
}[VaultWorkerMethod];

interface WorkerResult {
	[key: string]: unknown;
	encryptedChunk?: unknown;
	encryptedThumbnail?: unknown;
	chunkBytes?: unknown;
	thumbnailBytes?: unknown;
}

type WorkerResponse =
	{ id: number; ok: true; result: WorkerResult } | { id: number; ok: false; error: string };
type WorkerHandler<K extends VaultWorkerMethod> = (
	params: WorkerInput<K>,
) => WorkerResult | Promise<WorkerResult>;
type WorkerHandlers = { [K in VaultWorkerMethod]: WorkerHandler<K> };
type ActiveUpload = { fileKey: Uint8Array | null };

let unlockedMasterKey: Uint8Array | null = null;
let vaultGeneration = 0;
const encryption = new EncryptionService();
const decryption = new DecryptionService();
const hashes = new HashService();
const activeUploads = new Map<string, ActiveUpload>();
const activeReaders = new Map<string, Uint8Array | null>();
const activeShareReaders = new Map<string, Uint8Array | null>();

async function deriveVaultKEK(exportKey: Uint8Array, salt: Uint8Array): Promise<Uint8Array> {
	let rawKey: Uint8Array<ArrayBuffer> | null = new Uint8Array(exportKey);
	let rawSalt: Uint8Array<ArrayBuffer> | null = new Uint8Array(salt);

	try {
		return await hashes.hkdf(
			rawKey,
			rawSalt,
			new TextEncoder().encode('arkive:opaque-vault-kek:v1'),
		);
	} finally {
		rawKey = clearBytes(rawKey);
		rawSalt = clearBytes(rawSalt);
	}
}

function randomBytes(length: number): Uint8Array<ArrayBuffer> {
	return globalThis.crypto.getRandomValues(new Uint8Array(length));
}

function toUint8Array(value: ArrayBuffer | Uint8Array): Uint8Array {
	if (value instanceof Uint8Array) {
		return value;
	}

	return new Uint8Array(value);
}

function clearBytes(bytes: Uint8Array | null | undefined): null {
	if (bytes) {
		try {
			bytes.fill(0);
		} catch {
			// A detached buffer no longer exposes its contents to this worker.
		}
	}

	return null;
}

function aadBytes(value: unknown): Uint8Array {
	// Stable UTF-8 encoding keeps every client deriving the same authenticated context.
	if (!value) {
		return new Uint8Array();
	}
	if (value instanceof Uint8Array) {
		return value;
	}

	return new TextEncoder().encode(String(value));
}

function lockVault(): void {
	// WebCrypto cannot cancel an operation already in progress; invalidate its generation.
	vaultGeneration += 1;
	unlockedMasterKey = clearBytes(unlockedMasterKey);
	activeUploads.forEach(({ fileKey }) => clearBytes(fileKey));
	activeUploads.clear();
	activeReaders.forEach(clearBytes);
	activeReaders.clear();
	activeShareReaders.forEach(clearBytes);
	activeShareReaders.clear();
}

function assertVaultGeneration(generation: number): void {
	// Prevent in-flight operations from storing or returning keys after a lock.
	if (generation !== vaultGeneration) throw new Error('Vault was locked during this operation.');
}

function activeMasterKey(): Uint8Array {
	// Operations borrow this buffer; lockVault owns its lifetime and zeroizes it.
	if (!unlockedMasterKey) {
		throw new Error('Vault is locked');
	}

	return unlockedMasterKey;
}

async function wrapRecoveryMasterKey(
	masterKey: Uint8Array,
	recoveryKey: Uint8Array,
	userID: string,
): Promise<Uint8Array> {
	let kek: Uint8Array | null = null;

	try {
		kek = await hashes.hkdf(
			recoveryKey,
			new Uint8Array(32),
			new TextEncoder().encode('arkive-recovery-kek-v1'),
		);

		return await encryption.wrapKey(masterKey, kek, aadBytes(recoveryMasterKeyAAD(userID)));
	} finally {
		kek = clearBytes(kek);
	}
}

async function unwrapRecoveryMasterKey(
	encryptedMasterKey: Uint8Array,
	recoveryKey: Uint8Array,
	userID: string,
): Promise<Uint8Array> {
	let kek: Uint8Array | null = null;

	try {
		kek = await hashes.hkdf(
			recoveryKey,
			new Uint8Array(32),
			new TextEncoder().encode('arkive-recovery-kek-v1'),
		);

		return await decryption.unwrapKey(
			encryptedMasterKey,
			kek,
			aadBytes(recoveryMasterKeyAAD(userID)),
		);
	} catch (cause) {
		throw new Error('Invalid recovery key', { cause });
	} finally {
		kek = clearBytes(kek);
	}
}

async function handleUnlockVault(params: WorkerInput<'unlockVault'>) {
	const generation = vaultGeneration;
	let exportKey;
	let salt;
	let encryptedMasterKey;
	let kek;
	let masterKey;

	try {
		if (!params.exportKey) throw new Error('Missing vault export key');
		exportKey = toUint8Array(params.exportKey);
		params.exportKey = null;
		salt = fromBase64(params.salt);
		encryptedMasterKey = fromBase64(params.encryptedMasterKey);
		kek = await deriveVaultKEK(exportKey, salt);
		masterKey = await decryption.unwrapKey(encryptedMasterKey, kek, aadBytes(params.aad));
		assertVaultGeneration(generation);
		unlockedMasterKey = masterKey.slice();

		return { unlocked: true };
	} finally {
		exportKey = clearBytes(exportKey);
		salt = clearBytes(salt);
		encryptedMasterKey = clearBytes(encryptedMasterKey);
		kek = clearBytes(kek);
		masterKey = clearBytes(masterKey);
	}
}

async function handleCreateInitialVault(params: WorkerInput<'createInitialVault'>) {
	let exportKey;
	let salt;
	let masterKey;
	let kek;
	let encryptedMasterKey;

	try {
		if (!params.exportKey) throw new Error('Missing vault export key');
		exportKey = toUint8Array(params.exportKey);
		params.exportKey = null;
		salt = randomBytes(16);
		masterKey = randomBytes(32);
		kek = await deriveVaultKEK(exportKey, salt);
		encryptedMasterKey = await encryption.wrapKey(masterKey, kek, aadBytes(VAULT_MASTER_KEY_AAD));

		return {
			vaultSalt: toBase64(salt),
			encryptedMasterKey: toBase64(encryptedMasterKey),
			masterKey: masterKey.slice(),
		};
	} finally {
		exportKey = clearBytes(exportKey);
		salt = clearBytes(salt);
		masterKey = clearBytes(masterKey);
		kek = clearBytes(kek);
		encryptedMasterKey = clearBytes(encryptedMasterKey);
	}
}

async function handlePrepareInitialRecovery(params: WorkerInput<'prepareInitialRecovery'>) {
	const userID = String(params.userID || '').trim();

	if (!userID) throw new Error('Recovery setup context is unavailable');

	let masterKey;
	let recoveryKey;
	let encryptedMasterKeyRecovery;
	let recoveryProofPublicKey;

	try {
		if (!params.masterKeyBytes) throw new Error('Setup master key is unavailable');
		masterKey = toUint8Array(params.masterKeyBytes);
		params.masterKeyBytes = null;
		if (!masterKey.length) throw new Error('Setup master key is unavailable');
		recoveryKey = randomBytes(32);
		encryptedMasterKeyRecovery = await wrapRecoveryMasterKey(masterKey, recoveryKey, userID);
		recoveryProofPublicKey = await deriveRecoveryProofPublicKey(recoveryKey, userID);

		return {
			recoveryKey: await formatRecoveryKey(recoveryKey, hashes),
			encryptedMasterKeyRecovery: toBase64(encryptedMasterKeyRecovery),
			recoveryProofPublicKey: toBase64(recoveryProofPublicKey),
		};
	} finally {
		masterKey = clearBytes(masterKey);
		recoveryKey = clearBytes(recoveryKey);
		encryptedMasterKeyRecovery = clearBytes(encryptedMasterKeyRecovery);
		recoveryProofPublicKey = clearBytes(recoveryProofPublicKey);
	}
}

async function handleLockVault() {
	lockVault();

	return { unlocked: false };
}

async function handlePreparePasswordReset(params: WorkerInput<'preparePasswordReset'>) {
	let exportKey;
	let encryptedMasterKeyRecovery;
	let recoveryKeyBytes;
	let recoveryKeyText: string | null = null;
	let recoveryKey;
	let salt;
	let masterKey;
	let recoveryProof;
	let kek;
	let wrappedMasterKey;

	try {
		if (!params.exportKey) throw new Error('Missing vault export key');
		exportKey = toUint8Array(params.exportKey);
		params.exportKey = null;
		encryptedMasterKeyRecovery = fromBase64(params.encryptedMasterKeyRecovery);
		if (!params.recoveryKey) throw new Error('Recovery key is unavailable');
		recoveryKeyBytes = toUint8Array(params.recoveryKey);
		params.recoveryKey = null;
		recoveryKeyText = new TextDecoder().decode(recoveryKeyBytes);
		recoveryKeyBytes = clearBytes(recoveryKeyBytes);
		recoveryKey = await parseRecoveryKey(recoveryKeyText, hashes);
		recoveryKeyText = null;
		const userID = String(params.userID || '');
		salt = randomBytes(16);
		masterKey = await unwrapRecoveryMasterKey(encryptedMasterKeyRecovery, recoveryKey, userID);
		recoveryProof = await signRecoveryProof(recoveryKey, userID, String(params.resetToken || ''));
		kek = await deriveVaultKEK(exportKey, salt);
		wrappedMasterKey = await encryption.wrapKey(masterKey, kek, aadBytes(VAULT_MASTER_KEY_AAD));

		return {
			vaultSalt: toBase64(salt),
			encryptedMasterKey: toBase64(wrappedMasterKey),
			recoveryProof: toBase64(recoveryProof),
		};
	} finally {
		exportKey = clearBytes(exportKey);
		encryptedMasterKeyRecovery = clearBytes(encryptedMasterKeyRecovery);
		recoveryKeyBytes = clearBytes(recoveryKeyBytes);
		recoveryKeyText = null;
		recoveryKey = clearBytes(recoveryKey);
		salt = clearBytes(salt);
		masterKey = clearBytes(masterKey);
		recoveryProof = clearBytes(recoveryProof);
		kek = clearBytes(kek);
		wrappedMasterKey = clearBytes(wrappedMasterKey);
	}
}

async function handlePrepareShare(params: WorkerInput<'prepareShare'>) {
	if (!unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	let encryptedFileKey;
	let fileKey;
	let shareKey;
	let encryptedShareKey;
	let encryptedFileKeyForShare;

	try {
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			unlockedMasterKey,
			aadBytes(params.fileKeyAad),
		);
		shareKey = randomBytes(32);
		encryptedShareKey = await encryption.wrapKey(
			shareKey,
			unlockedMasterKey,
			aadBytes(params.shareKeyAad),
		);
		encryptedFileKeyForShare = await encryption.wrapKey(
			fileKey,
			shareKey,
			aadBytes(params.shareFileKeyAad),
		);

		return {
			encryptedShareKey: toBase64(encryptedShareKey),
			encryptedFileKeyForShare: toBase64(encryptedFileKeyForShare),
			shareSecret: shareKey.slice(),
			cryptoVersion: ENCRYPTION_VERSION,
		};
	} finally {
		encryptedFileKey = clearBytes(encryptedFileKey);
		fileKey = clearBytes(fileKey);
		shareKey = clearBytes(shareKey);
		encryptedShareKey = clearBytes(encryptedShareKey);
		encryptedFileKeyForShare = clearBytes(encryptedFileKeyForShare);
	}
}

async function handlePrepareFolderShare(params: WorkerInput<'prepareFolderShare'>) {
	if (!unlockedMasterKey) throw new Error('Vault is locked');
	const generation = vaultGeneration;
	let encryptedName;
	let encryptedMetadata;
	let name;
	let metadata;
	let shareKey;
	let encryptedShareKey;
	let shareName;
	let shareMetadata;

	try {
		encryptedName = fromBase64(params.encryptedName);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		name = await decryption.decrypt(
			encryptedName,
			unlockedMasterKey,
			aadBytes(folderNameAAD(params.vaultId, params.folderId, params.parentFolderId)),
		);
		metadata = await decryption.decrypt(
			encryptedMetadata,
			unlockedMasterKey,
			aadBytes(folderMetadataAAD(params.vaultId, params.folderId, params.parentFolderId)),
		);
		shareKey = randomBytes(32);
		encryptedShareKey = await encryption.wrapKey(
			shareKey,
			unlockedMasterKey,
			aadBytes(shareKeyAAD(params.token)),
		);
		shareName = await encryption.encrypt(
			name,
			shareKey,
			aadBytes(shareFolderNameAAD(params.folderId, params.token)),
		);
		shareMetadata = await encryption.encrypt(
			metadata,
			shareKey,
			aadBytes(shareFolderMetadataAAD(params.folderId, params.token)),
		);
		assertVaultGeneration(generation);

		return {
			encryptedShareKey: toBase64(encryptedShareKey),
			encryptedFolderName: toBase64(shareName),
			encryptedFolderMetadata: toBase64(shareMetadata),
			shareSecret: shareKey.slice(),
			cryptoVersion: ENCRYPTION_VERSION,
		};
	} finally {
		encryptedName = clearBytes(encryptedName);
		encryptedMetadata = clearBytes(encryptedMetadata);
		name = clearBytes(name);
		metadata = clearBytes(metadata);
		shareKey = clearBytes(shareKey);
		encryptedShareKey = clearBytes(encryptedShareKey);
		shareName = clearBytes(shareName);
		shareMetadata = clearBytes(shareMetadata);
	}
}

async function handleWrapFileKeyForShare(params: WorkerInput<'wrapFileKeyForShare'>) {
	if (!unlockedMasterKey) throw new Error('Vault is locked');
	let shareSecret;
	let encryptedFileKey;
	let fileKey;
	let wrapped;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			unlockedMasterKey,
			aadBytes(fileKeyAAD(params.vaultId, params.fileId, params.parentFolderId)),
		);
		wrapped = await encryption.wrapKey(
			fileKey,
			shareSecret,
			aadBytes(shareFileKeyAAD(params.fileId, params.token)),
		);

		return { encryptedFileKeyForShare: toBase64(wrapped) };
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		encryptedFileKey = clearBytes(encryptedFileKey);
		fileKey = clearBytes(fileKey);
		wrapped = clearBytes(wrapped);
	}
}

async function handleWrapUploadFileKeyForShare(params: WorkerInput<'wrapUploadFileKeyForShare'>) {
	const fileKey = activeUploads.get(String(params.uploadToken || ''))?.fileKey;

	if (!fileKey) throw new Error('Upload context is missing');
	let shareSecret;
	let wrapped;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		wrapped = await encryption.wrapKey(
			fileKey,
			shareSecret,
			aadBytes(shareFileKeyAAD(params.fileId, params.token)),
		);

		return { encryptedFileKeyForShare: toBase64(wrapped) };
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		wrapped = clearBytes(wrapped);
	}
}

async function handleWrapFolderForShare(params: WorkerInput<'wrapFolderForShare'>) {
	if (!unlockedMasterKey) throw new Error('Vault is locked');
	const generation = vaultGeneration;
	let shareSecret;
	let encryptedName;
	let encryptedMetadata;
	let name;
	let metadata;
	let wrappedName;
	let wrappedMetadata;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		encryptedName = fromBase64(params.encryptedName);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		name = await decryption.decrypt(
			encryptedName,
			unlockedMasterKey,
			aadBytes(folderNameAAD(params.vaultId, params.folderId, params.parentFolderId)),
		);
		metadata = await decryption.decrypt(
			encryptedMetadata,
			unlockedMasterKey,
			aadBytes(folderMetadataAAD(params.vaultId, params.folderId, params.parentFolderId)),
		);
		wrappedName = await encryption.encrypt(
			name,
			shareSecret,
			aadBytes(shareFolderNameAAD(params.folderId, params.token)),
		);
		wrappedMetadata = await encryption.encrypt(
			metadata,
			shareSecret,
			aadBytes(shareFolderMetadataAAD(params.folderId, params.token)),
		);
		assertVaultGeneration(generation);

		return { encryptedName: toBase64(wrappedName), encryptedMetadata: toBase64(wrappedMetadata) };
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		encryptedName = clearBytes(encryptedName);
		encryptedMetadata = clearBytes(encryptedMetadata);
		name = clearBytes(name);
		metadata = clearBytes(metadata);
		wrappedName = clearBytes(wrappedName);
		wrappedMetadata = clearBytes(wrappedMetadata);
	}
}

async function handleOpenPublicShareFolder(params: WorkerInput<'openPublicShareFolder'>) {
	let shareSecret;
	let encryptedName;
	let encryptedMetadata;
	let name;
	let metadata;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		encryptedName = fromBase64(params.encryptedName);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		name = await decryption.decrypt(
			encryptedName,
			shareSecret,
			aadBytes(shareFolderNameAAD(params.folderId, params.token)),
		);
		metadata = await decryption.decrypt(
			encryptedMetadata,
			shareSecret,
			aadBytes(shareFolderMetadataAAD(params.folderId, params.token)),
		);

		return { nameBytes: name.slice(), metadataBytes: metadata.slice() };
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		encryptedName = clearBytes(encryptedName);
		encryptedMetadata = clearBytes(encryptedMetadata);
		name = clearBytes(name);
		metadata = clearBytes(metadata);
	}
}

async function handleOpenPublicShareFileMetadata(
	params: WorkerInput<'openPublicShareFileMetadata'>,
) {
	let shareSecret;
	let encryptedFileKey;
	let encryptedMetadata;
	let fileKey;
	let metadata;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		encryptedFileKey = fromBase64(params.encryptedFileKeyForShare);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			shareSecret,
			aadBytes(shareFileKeyAAD(params.fileId, params.token)),
		);
		metadata = await decryption.decrypt(
			encryptedMetadata,
			fileKey,
			aadBytes(fileMetadataAAD(params.vaultId, params.fileId)),
		);

		return { metadataBytes: metadata.slice() };
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		encryptedFileKey = clearBytes(encryptedFileKey);
		encryptedMetadata = clearBytes(encryptedMetadata);
		fileKey = clearBytes(fileKey);
		metadata = clearBytes(metadata);
	}
}

async function handleOpenShareKey(params: WorkerInput<'openShareKey'>) {
	if (!unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	let encryptedShareKey;
	let shareKey;

	try {
		encryptedShareKey = fromBase64(params.encryptedShareKey);
		shareKey = await decryption.unwrapKey(
			encryptedShareKey,
			unlockedMasterKey,
			aadBytes(params.shareKeyAad),
		);

		return { shareSecret: shareKey.slice() };
	} finally {
		encryptedShareKey = clearBytes(encryptedShareKey);
		shareKey = clearBytes(shareKey);
	}
}

async function handlePrepareUpload(params: WorkerInput<'prepareUpload'>) {
	const uploadToken = String(params.uploadToken || '');

	if (!uploadToken) {
		throw new Error('Missing upload token');
	}
	if (!unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	const generation = vaultGeneration;
	let fileKey;
	let encryptedFileKey;

	try {
		fileKey = randomBytes(32);
		encryptedFileKey = await encryption.wrapKey(
			fileKey,
			unlockedMasterKey,
			aadBytes(params.fileKeyAad),
		);
		assertVaultGeneration(generation);
		activeUploads.set(uploadToken, { fileKey: fileKey.slice() });

		return {
			encryptedFileKey: toBase64(encryptedFileKey),
			totalParts: Number(params.totalParts || 0),
			encryptionVersion: ENCRYPTION_VERSION,
		};
	} finally {
		fileKey = clearBytes(fileKey);
		encryptedFileKey = clearBytes(encryptedFileKey);
	}
}

async function handleRewrapFileKey(params: WorkerInput<'rewrapFileKey'>) {
	if (!unlockedMasterKey) throw new Error('Vault is locked');
	const generation = vaultGeneration;
	let encryptedFileKey;
	let fileKey;
	let reboundFileKey;

	try {
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			unlockedMasterKey,
			aadBytes(params.oldAad),
		);
		assertVaultGeneration(generation);
		reboundFileKey = await encryption.wrapKey(fileKey, unlockedMasterKey, aadBytes(params.newAad));
		assertVaultGeneration(generation);

		return { encryptedFileKey: toBase64(reboundFileKey) };
	} finally {
		encryptedFileKey = clearBytes(encryptedFileKey);
		fileKey = clearBytes(fileKey);
		reboundFileKey = clearBytes(reboundFileKey);
	}
}

async function handleRewrapFolderMetadata(params: WorkerInput<'rewrapFolderMetadata'>) {
	if (!unlockedMasterKey) throw new Error('Vault is locked');
	const generation = vaultGeneration;
	let encryptedName;
	let encryptedMetadata;
	let name;
	let metadata;
	let reboundName;
	let reboundMetadata;

	try {
		encryptedName = fromBase64(params.encryptedName);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		name = await decryption.decrypt(encryptedName, unlockedMasterKey, aadBytes(params.oldNameAad));
		assertVaultGeneration(generation);
		metadata = await decryption.decrypt(
			encryptedMetadata,
			unlockedMasterKey,
			aadBytes(params.oldMetadataAad),
		);
		assertVaultGeneration(generation);
		reboundName = await encryption.encrypt(name, unlockedMasterKey, aadBytes(params.newNameAad));
		assertVaultGeneration(generation);
		reboundMetadata = await encryption.encrypt(
			metadata,
			unlockedMasterKey,
			aadBytes(params.newMetadataAad),
		);
		assertVaultGeneration(generation);

		return {
			encryptedName: toBase64(reboundName),
			encryptedMetadata: toBase64(reboundMetadata),
		};
	} finally {
		encryptedName = clearBytes(encryptedName);
		encryptedMetadata = clearBytes(encryptedMetadata);
		name = clearBytes(name);
		metadata = clearBytes(metadata);
		reboundName = clearBytes(reboundName);
		reboundMetadata = clearBytes(reboundMetadata);
	}
}

async function handleEncryptUploadMetadata(params: WorkerInput<'encryptUploadMetadata'>) {
	const uploadToken = String(params.uploadToken || '');
	let upload: ActiveUpload | undefined;
	let metadata;
	let encryptedMetadata;

	try {
		if (!params.metadataBytes) throw new Error('Metadata is unavailable');
		metadata = toUint8Array(params.metadataBytes);
		params.metadataBytes = null;
		upload = activeUploads.get(uploadToken);
		if (!upload || !upload.fileKey) throw new Error('Upload context is missing');
		encryptedMetadata = await encryption.encrypt(
			metadata,
			upload.fileKey,
			aadBytes(params.metadataAad),
		);

		return { encryptedMetadata: toBase64(encryptedMetadata) };
	} finally {
		params.metadataBytes = null;
		metadata = clearBytes(metadata);
		encryptedMetadata = clearBytes(encryptedMetadata);
	}
}

async function handleEncryptUploadThumbnail(params: WorkerInput<'encryptUploadThumbnail'>) {
	const uploadToken = String(params.uploadToken || '');
	let upload: ActiveUpload | undefined;
	let thumbnailBytes;
	let encryptedThumbnail;

	try {
		if (!params.thumbnailBytes) throw new Error('Thumbnail data is unavailable');
		thumbnailBytes = toUint8Array(params.thumbnailBytes);
		params.thumbnailBytes = null;
		upload = activeUploads.get(uploadToken);
		if (!upload || !upload.fileKey) throw new Error('Upload context is missing');
		encryptedThumbnail = await encryption.encrypt(
			thumbnailBytes,
			upload.fileKey,
			aadBytes(params.aad),
		);

		return {
			encryptedThumbnail: encryptedThumbnail.slice(),
			encryptedSize: encryptedThumbnail.length,
		};
	} finally {
		params.thumbnailBytes = null;
		thumbnailBytes = clearBytes(thumbnailBytes);
		encryptedThumbnail = clearBytes(encryptedThumbnail);
	}
}

async function handleEncryptUploadPart(params: WorkerInput<'encryptUploadPart'>) {
	const uploadToken = String(params.uploadToken || '');
	let upload: ActiveUpload | undefined;
	let chunkBytes;
	let encryptedChunk;
	let encryptedHash;

	try {
		if (!params.chunkBytes) throw new Error('Upload data is unavailable');
		chunkBytes = toUint8Array(params.chunkBytes);
		params.chunkBytes = null;
		upload = activeUploads.get(uploadToken);
		if (!upload || !upload.fileKey) throw new Error('Upload context is missing');
		encryptedChunk = await encryption.encrypt(chunkBytes, upload.fileKey, aadBytes(params.aad));
		encryptedHash = await hashes.sha256(encryptedChunk);

		return {
			encryptedChunk: encryptedChunk.slice(),
			encryptedHash: toBase64(encryptedHash),
			encryptedSize: encryptedChunk.length,
		};
	} finally {
		params.chunkBytes = null;
		chunkBytes = clearBytes(chunkBytes);
		encryptedChunk = clearBytes(encryptedChunk);
		encryptedHash = clearBytes(encryptedHash);
	}
}

async function handleHashUploadPayload(params: WorkerInput<'hashUploadPayload'>) {
	if (!params.bytes) throw new Error('Hash input is unavailable');
	let bytes: Uint8Array | null = toUint8Array(params.bytes);
	params.bytes = null;
	let hash;

	try {
		hash = await hashes.sha256(bytes);

		return { hash: toBase64(hash) };
	} finally {
		bytes = clearBytes(bytes);
		hash = clearBytes(hash);
	}
}

async function handleFinalizeUpload(params: WorkerInput<'finalizeUpload'>) {
	const uploadToken = String(params.uploadToken || '');
	let upload: ActiveUpload | undefined;
	let manifest;
	const decoded: Uint8Array[] = [];
	let encryptedManifest;
	let combined;
	let encryptedHash;

	try {
		upload = activeUploads.get(uploadToken);
		if (!upload || !upload.fileKey) throw new Error('Upload context is missing');
		if (!params.manifestBytes) throw new Error('Manifest is unavailable');
		manifest = toUint8Array(params.manifestBytes);
		params.manifestBytes = null;
		encryptedManifest = await encryption.encrypt(
			manifest,
			upload.fileKey,
			aadBytes(params.manifestAad),
		);
		const partHashes = params.partHashes || [];
		let totalLength = 0;

		for (let i = 0; i < partHashes.length; i += 1) {
			const hashBytes = fromBase64(partHashes[i]);
			decoded.push(hashBytes);
			totalLength += hashBytes.length;
		}
		combined = new Uint8Array(totalLength);
		let offset = 0;

		for (let i = 0; i < decoded.length; i += 1) {
			combined.set(decoded[i], offset);
			offset += decoded[i].length;
		}
		encryptedHash = await hashes.sha256(combined);

		return {
			encryptedManifest: toBase64(encryptedManifest),
			encryptedHash: toBase64(encryptedHash),
		};
	} finally {
		manifest = clearBytes(manifest);
		encryptedManifest = clearBytes(encryptedManifest);
		while (decoded.length) clearBytes(decoded.pop());
		combined = clearBytes(combined);
		encryptedHash = clearBytes(encryptedHash);
		if (upload) {
			upload.fileKey = clearBytes(upload.fileKey);
			activeUploads.delete(uploadToken);
		}
		params.manifestBytes = null;
	}
}

async function handleClearUploadContext(params: WorkerInput<'clearUploadContext'>) {
	const uploadToken = String(params.uploadToken || '');
	const upload = activeUploads.get(uploadToken);

	if (upload) {
		upload.fileKey = clearBytes(upload.fileKey);
		activeUploads.delete(uploadToken);
	}

	return { cleared: true };
}

async function handleOpenFileContext(params: WorkerInput<'openFileContext'>) {
	const contextID = String(params.contextId || '');

	if (!contextID) {
		throw new Error('Missing file context');
	}
	if (!unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	const generation = vaultGeneration;
	let encryptedFileKey;
	let encryptedMetadata;
	let encryptedManifest;
	let fileKey;
	let metadataBytes;
	let manifestBytes;

	try {
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		encryptedManifest = fromBase64(params.encryptedManifest);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			unlockedMasterKey,
			aadBytes(params.fileKeyAad),
		);
		assertVaultGeneration(generation);
		metadataBytes = await decryption.decrypt(
			encryptedMetadata,
			fileKey,
			aadBytes(params.metadataAad),
		);
		assertVaultGeneration(generation);
		manifestBytes = await decryption.decrypt(
			encryptedManifest,
			fileKey,
			aadBytes(params.manifestAad),
		);
		assertVaultGeneration(generation);
		activeReaders.set(contextID, fileKey.slice());

		return {
			metadataBytes: metadataBytes.slice(),
			manifestBytes: manifestBytes.slice(),
		};
	} finally {
		encryptedFileKey = clearBytes(encryptedFileKey);
		encryptedMetadata = clearBytes(encryptedMetadata);
		encryptedManifest = clearBytes(encryptedManifest);
		fileKey = clearBytes(fileKey);
		metadataBytes = clearBytes(metadataBytes);
		manifestBytes = clearBytes(manifestBytes);
	}
}

async function handleCloseFileContext(params: WorkerInput<'closeFileContext'>) {
	const contextID = String(params.contextId || '');
	let fileKey = activeReaders.get(contextID);

	if (fileKey) {
		fileKey = clearBytes(fileKey);
		activeReaders.delete(contextID);
	}

	return { cleared: true };
}

async function handleOpenPublicShareContext(params: WorkerInput<'openPublicShareContext'>) {
	const generation = vaultGeneration;
	const contextID = String(params.contextId || '');
	let shareSecret;
	let encryptedFileKeyForShare;
	let encryptedMetadata;
	let encryptedManifest;
	let fileKey;
	let metadataBytes;
	let manifestBytes;

	try {
		if (!params.shareSecret) throw new Error('Share secret is unavailable');
		shareSecret = toUint8Array(params.shareSecret);
		params.shareSecret = null;
		if (!contextID) throw new Error('Missing share context');
		encryptedFileKeyForShare = fromBase64(params.encryptedFileKeyForShare);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		encryptedManifest = fromBase64(params.encryptedManifest);
		fileKey = await decryption.unwrapKey(
			encryptedFileKeyForShare,
			shareSecret,
			aadBytes(params.shareFileKeyAad),
		);
		assertVaultGeneration(generation);
		metadataBytes = await decryption.decrypt(
			encryptedMetadata,
			fileKey,
			aadBytes(params.metadataAad),
		);
		assertVaultGeneration(generation);
		manifestBytes = await decryption.decrypt(
			encryptedManifest,
			fileKey,
			aadBytes(params.manifestAad),
		);
		assertVaultGeneration(generation);
		activeShareReaders.set(contextID, fileKey.slice());

		return {
			metadataBytes: metadataBytes.slice(),
			manifestBytes: manifestBytes.slice(),
		};
	} finally {
		params.shareSecret = null;
		shareSecret = clearBytes(shareSecret);
		encryptedFileKeyForShare = clearBytes(encryptedFileKeyForShare);
		encryptedMetadata = clearBytes(encryptedMetadata);
		encryptedManifest = clearBytes(encryptedManifest);
		fileKey = clearBytes(fileKey);
		metadataBytes = clearBytes(metadataBytes);
		manifestBytes = clearBytes(manifestBytes);
	}
}

async function handleClosePublicShareContext(params: WorkerInput<'closePublicShareContext'>) {
	const contextID = String(params.contextId || '');
	let fileKey = activeShareReaders.get(contextID);

	if (fileKey) {
		fileKey = clearBytes(fileKey);
		activeShareReaders.delete(contextID);
	}

	return { cleared: true };
}

async function handleDecryptFileChunk(params: WorkerInput<'decryptFileChunk'>) {
	const contextID = String(params.contextId || '');
	const fileKey = activeReaders.get(contextID);

	if (!fileKey) {
		throw new Error('File context is missing');
	}
	let encryptedChunk;
	let actualHash;
	let chunkBytes;

	try {
		if (!params.encryptedChunk) throw new Error('Encrypted chunk is unavailable');
		encryptedChunk = toUint8Array(params.encryptedChunk);
		params.encryptedChunk = null;
		const expectedHash = String(params.expectedHash || '');

		if (expectedHash) {
			actualHash = await hashes.sha256(encryptedChunk);
			if (toBase64(actualHash) !== expectedHash) {
				throw new Error('Encrypted chunk hash mismatch');
			}
		}
		chunkBytes = await decryption.decrypt(encryptedChunk, fileKey, aadBytes(params.aad));

		return { chunkBytes: chunkBytes.slice() };
	} finally {
		chunkBytes = clearBytes(chunkBytes);
		encryptedChunk = clearBytes(encryptedChunk);
		actualHash = clearBytes(actualHash);
	}
}

async function handleDecryptPublicShareChunk(params: WorkerInput<'decryptPublicShareChunk'>) {
	const contextID = String(params.contextId || '');
	const fileKey = activeShareReaders.get(contextID);

	if (!fileKey) {
		throw new Error('Share context is missing');
	}
	let encryptedChunk;
	let actualHash;
	let chunkBytes;

	try {
		if (!params.encryptedChunk) throw new Error('Encrypted chunk is unavailable');
		encryptedChunk = toUint8Array(params.encryptedChunk);
		params.encryptedChunk = null;
		const expectedHash = String(params.expectedHash || '');

		if (expectedHash) {
			actualHash = await hashes.sha256(encryptedChunk);
			const hashEncoding = String(params.hashEncoding || 'base64').toLowerCase();
			const actual =
				hashEncoding === 'hex'
					? Array.from(actualHash)
							.map(function (byte) {
								return byte.toString(16).padStart(2, '0');
							})
							.join('')
					: toBase64(actualHash);

			if (actual !== expectedHash) {
				throw new Error('Encrypted chunk hash mismatch');
			}
		}
		chunkBytes = await decryption.decrypt(encryptedChunk, fileKey, aadBytes(params.aad));

		return { chunkBytes: chunkBytes.slice() };
	} finally {
		chunkBytes = clearBytes(chunkBytes);
		encryptedChunk = clearBytes(encryptedChunk);
		actualHash = clearBytes(actualHash);
	}
}

async function handleEncryptFileMetadataInContext(
	params: WorkerInput<'encryptFileMetadataInContext'>,
) {
	const contextID = String(params.contextId || '');
	let fileKey: Uint8Array | null = null;
	let metadata;
	let encrypted;

	try {
		if (!params.metadataBytes) throw new Error('Metadata is unavailable');
		metadata = toUint8Array(params.metadataBytes);
		params.metadataBytes = null;
		fileKey = activeReaders.get(contextID) ?? null;
		if (!fileKey) throw new Error('File context is missing');
		encrypted = await encryption.encrypt(metadata, fileKey, aadBytes(params.aad));

		return { encryptedMetadata: toBase64(encrypted) };
	} finally {
		metadata = clearBytes(metadata);
		encrypted = clearBytes(encrypted);
		params.metadataBytes = null;
	}
}

async function handleEncryptFileMetadata(params: WorkerInput<'encryptFileMetadata'>) {
	let master: Uint8Array | null = null;
	let metadata;
	let encrypted;

	try {
		if (!params.metadataBytes) throw new Error('Metadata is unavailable');
		metadata = toUint8Array(params.metadataBytes);
		params.metadataBytes = null;
		master = unlockedMasterKey;
		if (!master) throw new Error('Vault is locked');
		encrypted = await encryption.encrypt(metadata, master, aadBytes(params.aad));

		return { encryptedMetadata: toBase64(encrypted) };
	} finally {
		metadata = clearBytes(metadata);
		encrypted = clearBytes(encrypted);
		params.metadataBytes = null;
	}
}

async function handleDecryptFileMetadata(params: WorkerInput<'decryptFileMetadata'>) {
	const master = activeMasterKey();
	let encryptedMetadata;
	let metadata;

	try {
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		metadata = await decryption.decrypt(encryptedMetadata, master, aadBytes(params.aad));

		return { metadataBytes: metadata.slice() };
	} finally {
		encryptedMetadata = clearBytes(encryptedMetadata);
		metadata = clearBytes(metadata);
	}
}

async function handleCreateSearchTokens(params: WorkerInput<'createSearchTokens'>) {
	const vaultId = String(params.vaultId || '');
	let terms: string[] = [];
	let searchKey;

	try {
		if (!unlockedMasterKey) throw new Error('Vault is locked');
		terms = Array.isArray(params.terms) ? params.terms : [];
		params.terms = null;
		if (!vaultId || !terms.length) return { tokens: [] };
		searchKey = await hashes.hkdf(
			unlockedMasterKey,
			new Uint8Array(32),
			new TextEncoder().encode('arkive-search-v1'),
		);
		const tokens = [];

		for (let i = 0; i < terms.length; i += 1) {
			let payload;
			let digest;

			try {
				payload = new TextEncoder().encode(vaultId + ':' + String(terms[i] || ''));
				digest = await hashes.hmacSha256(searchKey, payload);
				tokens.push(toBase64Url(digest));
			} finally {
				payload = clearBytes(payload);
				digest = clearBytes(digest);
			}
		}

		return { tokens: tokens };
	} finally {
		terms.length = 0;
		params.terms = null;
		searchKey = clearBytes(searchKey);
	}
}

async function handleDecryptSearchFile(params: WorkerInput<'decryptSearchFile'>) {
	if (!unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	let encryptedFileKey;
	let encryptedMetadata;
	let fileKey;
	let metadata;

	try {
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			unlockedMasterKey,
			aadBytes(params.fileKeyAad),
		);
		metadata = await decryption.decrypt(encryptedMetadata, fileKey, aadBytes(params.metadataAad));

		return { metadataBytes: metadata.slice() };
	} finally {
		fileKey = clearBytes(fileKey);
		encryptedFileKey = clearBytes(encryptedFileKey);
		encryptedMetadata = clearBytes(encryptedMetadata);
		metadata = clearBytes(metadata);
	}
}

async function handleDecryptSearchFolder(params: WorkerInput<'decryptSearchFolder'>) {
	if (!unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	let encryptedName;
	let encryptedMetadata;
	let name;
	let metadata;

	try {
		encryptedName = fromBase64(params.encryptedName);
		encryptedMetadata = fromBase64(params.encryptedMetadata);
		name = await decryption.decrypt(encryptedName, unlockedMasterKey, aadBytes(params.nameAad));
		metadata = await decryption.decrypt(
			encryptedMetadata,
			unlockedMasterKey,
			aadBytes(params.metadataAad),
		);

		return { nameBytes: name.slice(), metadataBytes: metadata.slice() };
	} finally {
		encryptedName = clearBytes(encryptedName);
		encryptedMetadata = clearBytes(encryptedMetadata);
		name = clearBytes(name);
		metadata = clearBytes(metadata);
	}
}

async function handleDecryptSearchThumbnail(params: WorkerInput<'decryptSearchThumbnail'>) {
	if (!unlockedMasterKey) {
		throw new Error('Vault is locked');
	}
	let encryptedFileKey;
	let encryptedThumbnail;
	let fileKey;
	let thumbnail;

	try {
		if (!params.encryptedThumbnail) throw new Error('Encrypted thumbnail is unavailable');
		encryptedFileKey = fromBase64(params.encryptedFileKey);
		encryptedThumbnail = toUint8Array(params.encryptedThumbnail);
		params.encryptedThumbnail = null;
		fileKey = await decryption.unwrapKey(
			encryptedFileKey,
			unlockedMasterKey,
			aadBytes(params.fileKeyAad),
		);
		thumbnail = await decryption.decrypt(
			encryptedThumbnail,
			fileKey,
			aadBytes(params.thumbnailAad),
		);

		return { thumbnailBytes: thumbnail.slice() };
	} finally {
		encryptedFileKey = clearBytes(encryptedFileKey);
		encryptedThumbnail = clearBytes(encryptedThumbnail);
		fileKey = clearBytes(fileKey);
		thumbnail = clearBytes(thumbnail);
	}
}

const messageHandlers: WorkerHandlers = {
	createInitialVault: handleCreateInitialVault,
	prepareInitialRecovery: handlePrepareInitialRecovery,
	unlockVault: handleUnlockVault,
	lockVault: handleLockVault,
	preparePasswordReset: handlePreparePasswordReset,
	prepareShare: handlePrepareShare,
	prepareFolderShare: handlePrepareFolderShare,
	wrapFileKeyForShare: handleWrapFileKeyForShare,
	wrapUploadFileKeyForShare: handleWrapUploadFileKeyForShare,
	wrapFolderForShare: handleWrapFolderForShare,
	openPublicShareFolder: handleOpenPublicShareFolder,
	openPublicShareFileMetadata: handleOpenPublicShareFileMetadata,
	openShareKey: handleOpenShareKey,
	prepareUpload: handlePrepareUpload,
	rewrapFileKey: handleRewrapFileKey,
	rewrapFolderMetadata: handleRewrapFolderMetadata,
	encryptUploadMetadata: handleEncryptUploadMetadata,
	encryptUploadThumbnail: handleEncryptUploadThumbnail,
	encryptUploadPart: handleEncryptUploadPart,
	hashUploadPayload: handleHashUploadPayload,
	finalizeUpload: handleFinalizeUpload,
	clearUploadContext: handleClearUploadContext,
	openFileContext: handleOpenFileContext,
	closeFileContext: handleCloseFileContext,
	openPublicShareContext: handleOpenPublicShareContext,
	closePublicShareContext: handleClosePublicShareContext,
	decryptFileChunk: handleDecryptFileChunk,
	decryptPublicShareChunk: handleDecryptPublicShareChunk,
	encryptFileMetadataInContext: handleEncryptFileMetadataInContext,
	encryptFileMetadata: handleEncryptFileMetadata,
	encryptFolderMetadata: handleEncryptFileMetadata,
	decryptFileMetadata: handleDecryptFileMetadata,
	decryptFolderMetadata: handleDecryptFileMetadata,
	createSearchTokens: handleCreateSearchTokens,
	decryptSearchFile: handleDecryptSearchFile,
	decryptSearchFolder: handleDecryptSearchFolder,
	decryptSearchThumbnail: handleDecryptSearchThumbnail,
};

async function handleMessage(message: WorkerRequest): Promise<WorkerResult> {
	// Map lookup loses the method/params correlation; WorkerRequest preserves it.
	const handler = messageHandlers[message.method] as WorkerHandler<VaultWorkerMethod>;

	return handler(message.params);
}

const workerScope = self as unknown as {
	addEventListener(type: 'message', listener: (event: MessageEvent<WorkerRequest>) => void): void;
	postMessage(message: WorkerResponse, transfer?: Transferable[]): void;
};

function clearWorkerResult(result: WorkerResult): void {
	// Results can hold plaintext byte arrays or recovery-key text that must be dropped on lock.
	for (const [key, value] of Object.entries(result)) {
		if (value instanceof Uint8Array) value.fill(0);
		else if (Array.isArray(value)) value.fill('');
		result[key] = null;
	}
}

workerScope.addEventListener('message', async (event) => {
	const payload = event.data;

	// WebCrypto is asynchronous; discard outputs from work that finishes after a lock.
	if (payload.method === 'unlockVault') lockVault();
	const generation = vaultGeneration;
	let result: WorkerResult | undefined;

	try {
		result = await handleMessage(payload);
		if (payload.method !== 'lockVault' && generation !== vaultGeneration) {
			clearWorkerResult(result);
			result = undefined;

			throw new Error('Vault was locked during this operation.');
		}
		const transfer: Transferable[] = [];

		for (const value of [
			result.masterKey,
			result.shareSecret,
			result.encryptedChunk,
			result.encryptedThumbnail,
			result.chunkBytes,
			result.thumbnailBytes,
			result.metadataBytes,
			result.manifestBytes,
			result.nameBytes,
		]) {
			if (value instanceof Uint8Array && value.buffer instanceof ArrayBuffer)
				transfer.push(value.buffer);
		}
		workerScope.postMessage({ id: payload.id, ok: true, result }, transfer);
		result = undefined;
	} catch (error) {
		if (result) clearWorkerResult(result);
		workerScope.postMessage({
			id: payload.id,
			ok: false,
			error: error instanceof Error ? error.message : 'Vault worker failed',
		});
	}
});
