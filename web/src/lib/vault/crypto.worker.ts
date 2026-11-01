import initArkiveCrypto, * as arkiveCrypto from '../../../../core/web/static/vendor/arkive-crypto/arkive_crypto.js';
import { deriveRecoveryProofPublicKey, signRecoveryProof } from '../recovery-proof';
import { fromBase64, toBase64, toBase64Url } from './bytes';
import type { VaultWorkerCalls, VaultWorkerMethod } from './types';

type CryptoApi = typeof arkiveCrypto;

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

type WorkerResponse = { id: number; ok: true; result: WorkerResult } | { id: number; ok: false; error: string };
type WorkerHandler<K extends VaultWorkerMethod> = (params: WorkerInput<K>, crypto: CryptoApi) => WorkerResult | Promise<WorkerResult>;
type WorkerHandlers = { [K in VaultWorkerMethod]: WorkerHandler<K> };
type ActiveUpload = { fileKey: Uint8Array | null };

let readyPromise: Promise<CryptoApi> | null = null;
let unlockedMasterKey: Uint8Array | null = null;
const activeUploads = new Map<string, ActiveUpload>();
const activeReaders = new Map<string, Uint8Array | null>();
const activeShareReaders = new Map<string, Uint8Array | null>();

function ensureCrypto(): Promise<CryptoApi> {
  if (readyPromise) {
    return readyPromise;
  }
  readyPromise = initArkiveCrypto().then(function () {
    return arkiveCrypto;
  });
  return readyPromise;
}

async function deriveVaultKEK(crypto: CryptoApi, exportKey: Uint8Array, salt: Uint8Array): Promise<Uint8Array> {
  let rawKey: Uint8Array<ArrayBuffer> | null = new Uint8Array(exportKey);
  let rawSalt: Uint8Array<ArrayBuffer> | null = new Uint8Array(salt);
  try {
    const key = await globalThis.crypto.subtle.importKey('raw', rawKey, 'HKDF', false, ['deriveBits']);
    const bits = await globalThis.crypto.subtle.deriveBits(
      {
        name: 'HKDF',
        hash: 'SHA-256',
        salt: rawSalt,
        info: new TextEncoder().encode('arkive:opaque-vault-kek:v1'),
      },
      key,
      256,
    );
    return new Uint8Array(bits);
  } finally {
    rawKey = clearBytes(crypto, rawKey);
    rawSalt = clearBytes(crypto, rawSalt);
  }
}

function toUint8Array(value: ArrayBuffer | Uint8Array): Uint8Array {
  if (value instanceof Uint8Array) {
    return value;
  }
  return new Uint8Array(value);
}

function clearBytes(crypto: CryptoApi, bytes: Uint8Array | null | undefined): null {
	if (bytes) {
		try {
			crypto.zeroize(bytes);
		} catch {
			// The JavaScript fill below remains the fallback if WASM cannot clear the view.
		}
		try {
			bytes.fill(0);
		} catch {
			// A detached buffer no longer exposes its contents to this worker.
		}
	}
	return null;
}

function aadBytes(value: unknown): Uint8Array {
  if (!value) {
    return new Uint8Array();
  }
  if (value instanceof Uint8Array) {
    return value;
  }
  return new TextEncoder().encode(String(value));
}

function lockVault(crypto: CryptoApi): void {
  unlockedMasterKey = clearBytes(crypto, unlockedMasterKey);
  activeUploads.forEach(function(fileKey) {
    fileKey.fileKey = clearBytes(crypto, fileKey.fileKey);
  });
  activeUploads.clear();
  activeReaders.forEach((fileKey, id) => activeReaders.set(id, clearBytes(crypto, fileKey)));
  activeReaders.clear();
  activeShareReaders.forEach((fileKey, id) => activeShareReaders.set(id, clearBytes(crypto, fileKey)));
  activeShareReaders.clear();
}

function activeMasterKey(): Uint8Array {
	// Operations borrow this buffer; lockVault owns its lifetime and zeroizes it.
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  return unlockedMasterKey;
}

function parseRecoveryKey(crypto: CryptoApi, value: unknown): Uint8Array {
  try {
    return crypto.parse_recovery_key(String(value || "").trim());
  } catch (_) {
    throw new Error("Invalid recovery key");
  }
}

function unwrapRecoveryMasterKey(
  crypto: CryptoApi,
  encryptedMasterKey: Uint8Array,
  recoveryKey: Uint8Array,
  userID: string,
): Uint8Array {
  try {
    return crypto.unwrap_master_key_with_recovery_key(
      encryptedMasterKey,
      recoveryKey,
      userID,
    );
  } catch (_) {
    throw new Error("Invalid recovery key");
  }
}

async function handleUnlockVault(params: WorkerInput<'unlockVault'>, crypto: CryptoApi) {
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
		kek = await deriveVaultKEK(crypto, exportKey, salt);
    masterKey = crypto.unwrap_master_key(
      encryptedMasterKey,
      kek,
      aadBytes(params.aad),
    );
    lockVault(crypto);
    unlockedMasterKey = masterKey.slice();
    return { unlocked: true };
  } finally {
    exportKey = clearBytes(crypto, exportKey);
    salt = clearBytes(crypto, salt);
    encryptedMasterKey = clearBytes(crypto, encryptedMasterKey);
    kek = clearBytes(crypto, kek);
    masterKey = clearBytes(crypto, masterKey);
  }
}

async function handleCreateInitialVault(params: WorkerInput<'createInitialVault'>, crypto: CryptoApi) {
  let exportKey;
  let salt;
  let masterKey;
  let kek;
  let encryptedMasterKey;
  try {
    if (!params.exportKey) throw new Error('Missing vault export key');
    exportKey = toUint8Array(params.exportKey);
    params.exportKey = null;
    salt = crypto.generate_salt();
    masterKey = crypto.generate_master_key();
		kek = await deriveVaultKEK(crypto, exportKey, salt);
    encryptedMasterKey = crypto.wrap_master_key(
      masterKey,
      kek,
      new TextEncoder().encode('arkive:master-key:v1'),
    );
    return {
      vaultSalt: toBase64(salt),
      encryptedMasterKey: toBase64(encryptedMasterKey),
      masterKey: masterKey.slice(),
    };
  } finally {
    exportKey = clearBytes(crypto, exportKey);
    salt = clearBytes(crypto, salt);
    masterKey = clearBytes(crypto, masterKey);
    kek = clearBytes(crypto, kek);
    encryptedMasterKey = clearBytes(crypto, encryptedMasterKey);
  }
}

async function handlePrepareInitialRecovery(params: WorkerInput<'prepareInitialRecovery'>, crypto: CryptoApi) {
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
    recoveryKey = crypto.generate_recovery_key();
    encryptedMasterKeyRecovery = crypto.wrap_master_key_with_recovery_key(masterKey, recoveryKey, userID);
    recoveryProofPublicKey = await deriveRecoveryProofPublicKey(recoveryKey, userID);
    return {
      recoveryKey: crypto.format_recovery_key(recoveryKey),
      encryptedMasterKeyRecovery: toBase64(encryptedMasterKeyRecovery),
      recoveryProofPublicKey: toBase64(recoveryProofPublicKey),
    };
  } finally {
    masterKey = clearBytes(crypto, masterKey);
    recoveryKey = clearBytes(crypto, recoveryKey);
    encryptedMasterKeyRecovery = clearBytes(crypto, encryptedMasterKeyRecovery);
    recoveryProofPublicKey = clearBytes(crypto, recoveryProofPublicKey);
  }
}

function handleLockVault(params: WorkerInput<'lockVault'>, crypto: CryptoApi) {
  lockVault(crypto);
  return { unlocked: false };
}

async function handlePreparePasswordReset(params: WorkerInput<'preparePasswordReset'>, crypto: CryptoApi) {
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
    encryptedMasterKeyRecovery = fromBase64(
      params.encryptedMasterKeyRecovery,
    );
    if (!params.recoveryKey) throw new Error('Recovery key is unavailable');
    recoveryKeyBytes = toUint8Array(params.recoveryKey);
    params.recoveryKey = null;
    recoveryKeyText = new TextDecoder().decode(recoveryKeyBytes);
    recoveryKeyBytes = clearBytes(crypto, recoveryKeyBytes);
    recoveryKey = parseRecoveryKey(crypto, recoveryKeyText);
    recoveryKeyText = null;
    const userID = String(params.userID || "");
    salt = crypto.generate_salt();
    masterKey = unwrapRecoveryMasterKey(
      crypto,
      encryptedMasterKeyRecovery,
      recoveryKey,
      userID,
    );
    recoveryProof = await signRecoveryProof(
      recoveryKey,
      userID,
      String(params.resetToken || ""),
    );
		kek = await deriveVaultKEK(crypto, exportKey, salt);
    wrappedMasterKey = crypto.wrap_master_key(
      masterKey,
      kek,
      aadBytes("arkive:master-key:v1"),
    );
    return {
      vaultSalt: toBase64(salt),
      encryptedMasterKey: toBase64(wrappedMasterKey),
      recoveryProof: toBase64(recoveryProof),
    };
  } finally {
    exportKey = clearBytes(crypto, exportKey);
    encryptedMasterKeyRecovery = clearBytes(crypto, encryptedMasterKeyRecovery);
    recoveryKeyBytes = clearBytes(crypto, recoveryKeyBytes);
    recoveryKeyText = null;
    recoveryKey = clearBytes(crypto, recoveryKey);
    salt = clearBytes(crypto, salt);
    masterKey = clearBytes(crypto, masterKey);
    recoveryProof = clearBytes(crypto, recoveryProof);
    kek = clearBytes(crypto, kek);
    wrappedMasterKey = clearBytes(crypto, wrappedMasterKey);
  }
}

function handlePrepareShare(params: WorkerInput<'prepareShare'>, crypto: CryptoApi) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let encryptedFileKey;
  let fileKey;
  let shareKey;
  let encryptedShareKey;
  let encryptedFileKeyForShare;
  try {
    encryptedFileKey = fromBase64(params.encryptedFileKey);
    fileKey = crypto.unwrap_file_key(
      encryptedFileKey,
      unlockedMasterKey,
      aadBytes(params.fileKeyAad),
    );
    shareKey = crypto.generate_share_key();
    encryptedShareKey = crypto.wrap_file_key(
      shareKey,
      unlockedMasterKey,
      aadBytes(params.shareKeyAad),
    );
    encryptedFileKeyForShare = crypto.wrap_file_key(
      fileKey,
      shareKey,
      aadBytes(params.shareFileKeyAad),
    );
    return {
      encryptedShareKey: toBase64(encryptedShareKey),
      encryptedFileKeyForShare: toBase64(encryptedFileKeyForShare),
      shareSecret: shareKey.slice(),
      cryptoVersion: 1,
    };
  } finally {
    encryptedFileKey = clearBytes(crypto, encryptedFileKey);
    fileKey = clearBytes(crypto, fileKey);
    shareKey = clearBytes(crypto, shareKey);
    encryptedShareKey = clearBytes(crypto, encryptedShareKey);
    encryptedFileKeyForShare = clearBytes(crypto, encryptedFileKeyForShare);
  }
}

function handleOpenShareKey(params: WorkerInput<'openShareKey'>, crypto: CryptoApi) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let encryptedShareKey;
  let shareKey;
  try {
    encryptedShareKey = fromBase64(params.encryptedShareKey);
    shareKey = crypto.unwrap_file_key(
      encryptedShareKey,
      unlockedMasterKey,
      aadBytes(params.shareKeyAad),
    );
    return { shareSecret: shareKey.slice() };
  } finally {
    encryptedShareKey = clearBytes(crypto, encryptedShareKey);
    shareKey = clearBytes(crypto, shareKey);
  }
}

function handlePrepareUpload(params: WorkerInput<'prepareUpload'>, crypto: CryptoApi) {
  const uploadToken = String(params.uploadToken || "");
  if (!uploadToken) {
    throw new Error("Missing upload token");
  }
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let fileKey;
  let encryptedFileKey;
  try {
    fileKey = crypto.generate_file_key();
    encryptedFileKey = crypto.wrap_file_key(
      fileKey,
      unlockedMasterKey,
      aadBytes(params.fileKeyAad),
    );
    activeUploads.set(uploadToken, { fileKey: fileKey.slice() });
    return {
      encryptedFileKey: toBase64(encryptedFileKey),
      totalParts: Number(params.totalParts || 0),
      encryptionVersion: 1,
    };
  } finally {
    fileKey = clearBytes(crypto, fileKey);
		encryptedFileKey = clearBytes(crypto, encryptedFileKey);
  }
}

function handleEncryptUploadMetadata(params: WorkerInput<'encryptUploadMetadata'>, crypto: CryptoApi) {
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
		encryptedMetadata = crypto.encrypt_chunk(metadata, upload.fileKey, aadBytes(params.metadataAad));
		return { encryptedMetadata: toBase64(encryptedMetadata) };
	} finally {
		params.metadataBytes = null;
		metadata = clearBytes(crypto, metadata);
		encryptedMetadata = clearBytes(crypto, encryptedMetadata);
	}
}

function handleEncryptUploadThumbnail(params: WorkerInput<'encryptUploadThumbnail'>, crypto: CryptoApi) {
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
		encryptedThumbnail = crypto.encrypt_chunk(thumbnailBytes, upload.fileKey, aadBytes(params.aad));
		return {
			encryptedThumbnail: encryptedThumbnail.slice(),
			encryptedSize: encryptedThumbnail.length
		};
	} finally {
		params.thumbnailBytes = null;
		thumbnailBytes = clearBytes(crypto, thumbnailBytes);
		encryptedThumbnail = clearBytes(crypto, encryptedThumbnail);
	}
}

function handleEncryptUploadPart(params: WorkerInput<'encryptUploadPart'>, crypto: CryptoApi) {
  const uploadToken = String(params.uploadToken || "");
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
    encryptedChunk = crypto.encrypt_chunk(
      chunkBytes,
      upload.fileKey,
      aadBytes(params.aad),
    );
    encryptedHash = crypto.hash_bytes_blake3(encryptedChunk);
    return {
      encryptedChunk: encryptedChunk.slice(),
      encryptedHash: toBase64(encryptedHash),
      encryptedSize: encryptedChunk.length,
    };
  } finally {
    params.chunkBytes = null;
    chunkBytes = clearBytes(crypto, chunkBytes);
	encryptedChunk = clearBytes(crypto, encryptedChunk);
    encryptedHash = clearBytes(crypto, encryptedHash);
  }
}

function handleHashUploadPayload(params: WorkerInput<'hashUploadPayload'>, crypto: CryptoApi) {
	if (!params.bytes) throw new Error('Hash input is unavailable');
	let bytes: Uint8Array | null = toUint8Array(params.bytes);
	params.bytes = null;
	let hash;
	try {
		hash = crypto.hash_bytes_blake3(bytes);
		return { hash: toBase64(hash) };
	} finally {
		bytes = clearBytes(crypto, bytes);
		hash = clearBytes(crypto, hash);
	}
}

function handleFinalizeUpload(params: WorkerInput<'finalizeUpload'>, crypto: CryptoApi) {
  const uploadToken = String(params.uploadToken || "");
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
    encryptedManifest = crypto.encrypt_chunk(
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
    encryptedHash = crypto.hash_bytes_blake3(combined);
    return {
      encryptedManifest: toBase64(encryptedManifest),
      encryptedHash: toBase64(encryptedHash),
    };
  } finally {
    manifest = clearBytes(crypto, manifest);
    encryptedManifest = clearBytes(crypto, encryptedManifest);
    while (decoded.length) clearBytes(crypto, decoded.pop());
    combined = clearBytes(crypto, combined);
    encryptedHash = clearBytes(crypto, encryptedHash);
		if (upload) {
			upload.fileKey = clearBytes(crypto, upload.fileKey);
			activeUploads.delete(uploadToken);
		}
		params.manifestBytes = null;
  }
}

function handleClearUploadContext(params: WorkerInput<'clearUploadContext'>, crypto: CryptoApi) {
  const uploadToken = String(params.uploadToken || "");
  const upload = activeUploads.get(uploadToken);
  if (upload) {
    upload.fileKey = clearBytes(crypto, upload.fileKey);
    activeUploads.delete(uploadToken);
  }
  return { cleared: true };
}

function handleOpenFileContext(params: WorkerInput<'openFileContext'>, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  if (!contextID) {
    throw new Error("Missing file context");
  }
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
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
    fileKey = crypto.unwrap_file_key(
      encryptedFileKey,
      unlockedMasterKey,
      aadBytes(params.fileKeyAad),
    );
    metadataBytes = crypto.decrypt_chunk(
      encryptedMetadata,
      fileKey,
      aadBytes(params.metadataAad),
    );
    manifestBytes = crypto.decrypt_chunk(
      encryptedManifest,
      fileKey,
      aadBytes(params.manifestAad),
    );
    activeReaders.set(contextID, fileKey.slice());
    return {
      metadataBytes: metadataBytes.slice(),
      manifestBytes: manifestBytes.slice(),
    };
  } finally {
    encryptedFileKey = clearBytes(crypto, encryptedFileKey);
    encryptedMetadata = clearBytes(crypto, encryptedMetadata);
    encryptedManifest = clearBytes(crypto, encryptedManifest);
    fileKey = clearBytes(crypto, fileKey);
    metadataBytes = clearBytes(crypto, metadataBytes);
    manifestBytes = clearBytes(crypto, manifestBytes);
  }
}

function handleCloseFileContext(params: WorkerInput<'closeFileContext'>, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  let fileKey = activeReaders.get(contextID);
  if (fileKey) {
    fileKey = clearBytes(crypto, fileKey);
    activeReaders.delete(contextID);
  }
  return { cleared: true };
}

function handleOpenPublicShareContext(params: WorkerInput<'openPublicShareContext'>, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
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
    fileKey = crypto.unwrap_file_key(
      encryptedFileKeyForShare,
      shareSecret,
      aadBytes(params.shareFileKeyAad),
    );
    metadataBytes = crypto.decrypt_chunk(
      encryptedMetadata,
      fileKey,
      aadBytes(params.metadataAad),
    );
    manifestBytes = crypto.decrypt_chunk(
      encryptedManifest,
      fileKey,
      aadBytes(params.manifestAad),
    );
    activeShareReaders.set(contextID, fileKey.slice());
    return {
      metadataBytes: metadataBytes.slice(),
      manifestBytes: manifestBytes.slice(),
    };
  } finally {
    params.shareSecret = null;
    shareSecret = clearBytes(crypto, shareSecret);
    encryptedFileKeyForShare = clearBytes(crypto, encryptedFileKeyForShare);
    encryptedMetadata = clearBytes(crypto, encryptedMetadata);
    encryptedManifest = clearBytes(crypto, encryptedManifest);
    fileKey = clearBytes(crypto, fileKey);
    metadataBytes = clearBytes(crypto, metadataBytes);
    manifestBytes = clearBytes(crypto, manifestBytes);
  }
}

function handleClosePublicShareContext(params: WorkerInput<'closePublicShareContext'>, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  let fileKey = activeShareReaders.get(contextID);
  if (fileKey) {
    fileKey = clearBytes(crypto, fileKey);
    activeShareReaders.delete(contextID);
  }
  return { cleared: true };
}

function handleDecryptFileChunk(params: WorkerInput<'decryptFileChunk'>, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  const fileKey = activeReaders.get(contextID);
  if (!fileKey) {
    throw new Error("File context is missing");
  }
  let encryptedChunk;
  let actualHash;
  let chunkBytes;
  try {
		if (!params.encryptedChunk) throw new Error('Encrypted chunk is unavailable');
    encryptedChunk = toUint8Array(params.encryptedChunk);
		params.encryptedChunk = null;
    const expectedHash = String(params.expectedHash || "");
    if (expectedHash) {
      actualHash = crypto.hash_bytes_blake3(encryptedChunk);
      if (toBase64(actualHash) !== expectedHash) {
        throw new Error("Encrypted chunk hash mismatch");
      }
    }
    chunkBytes = crypto.decrypt_chunk(
      encryptedChunk,
      fileKey,
      aadBytes(params.aad),
    );
    return { chunkBytes: chunkBytes.slice() };
  } finally {
    chunkBytes = clearBytes(crypto, chunkBytes);
	encryptedChunk = clearBytes(crypto, encryptedChunk);
	actualHash = clearBytes(crypto, actualHash);
  }
}

function handleDecryptPublicShareChunk(params: WorkerInput<'decryptPublicShareChunk'>, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  const fileKey = activeShareReaders.get(contextID);
  if (!fileKey) {
    throw new Error("Share context is missing");
  }
  let encryptedChunk;
  let actualHash;
  let chunkBytes;
  try {
		if (!params.encryptedChunk) throw new Error('Encrypted chunk is unavailable');
    encryptedChunk = toUint8Array(params.encryptedChunk);
		params.encryptedChunk = null;
    const expectedHash = String(params.expectedHash || "");
    if (expectedHash) {
      actualHash = crypto.hash_bytes_blake3(encryptedChunk);
      const hashEncoding = String(params.hashEncoding || "base64").toLowerCase();
      const actual = hashEncoding === "hex"
        ? Array.from(actualHash).map(function(byte) {
          return byte.toString(16).padStart(2, "0");
        }).join("")
        : toBase64(actualHash);
      if (actual !== expectedHash) {
        throw new Error("Encrypted chunk hash mismatch");
      }
    }
    chunkBytes = crypto.decrypt_chunk(
      encryptedChunk,
      fileKey,
      aadBytes(params.aad),
    );
    return { chunkBytes: chunkBytes.slice() };
  } finally {
    chunkBytes = clearBytes(crypto, chunkBytes);
	encryptedChunk = clearBytes(crypto, encryptedChunk);
	actualHash = clearBytes(crypto, actualHash);
  }
}

function handleEncryptFileMetadataInContext(params: WorkerInput<'encryptFileMetadataInContext'>, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  let fileKey: Uint8Array | null = null;
  let metadata;
  let encrypted;
  try {
		if (!params.metadataBytes) throw new Error('Metadata is unavailable');
		metadata = toUint8Array(params.metadataBytes);
		params.metadataBytes = null;
		fileKey = activeReaders.get(contextID) ?? null;
		if (!fileKey) throw new Error('File context is missing');
    encrypted = crypto.encrypt_chunk(
      metadata,
      fileKey,
      aadBytes(params.aad),
    );
    return { encryptedMetadata: toBase64(encrypted) };
  } finally {
    metadata = clearBytes(crypto, metadata);
    encrypted = clearBytes(crypto, encrypted);
		params.metadataBytes = null;
  }
}

function handleEncryptFileMetadata(params: WorkerInput<'encryptFileMetadata'>, crypto: CryptoApi) {
	let master: Uint8Array | null = null;
  let metadata;
  let encrypted;
  try {
		if (!params.metadataBytes) throw new Error('Metadata is unavailable');
		metadata = toUint8Array(params.metadataBytes);
		params.metadataBytes = null;
		master = unlockedMasterKey;
		if (!master) throw new Error('Vault is locked');
    encrypted = crypto.encrypt_chunk(
      metadata,
      master,
      aadBytes(params.aad),
    );
    return { encryptedMetadata: toBase64(encrypted) };
  } finally {
    metadata = clearBytes(crypto, metadata);
    encrypted = clearBytes(crypto, encrypted);
		params.metadataBytes = null;
  }
}

function handleDecryptFileMetadata(params: WorkerInput<'decryptFileMetadata'>, crypto: CryptoApi) {
	const master = activeMasterKey();
  let encryptedMetadata;
  let metadata;
  try {
    encryptedMetadata = fromBase64(params.encryptedMetadata);
    metadata = crypto.decrypt_chunk(
      encryptedMetadata,
      master,
      aadBytes(params.aad),
    );
		return { metadataBytes: metadata.slice() };
  } finally {
    encryptedMetadata = clearBytes(crypto, encryptedMetadata);
    metadata = clearBytes(crypto, metadata);
  }
}

function handleCreateSearchTokens(params: WorkerInput<'createSearchTokens'>, crypto: CryptoApi) {
  const vaultId = String(params.vaultId || "");
  let terms: string[] = [];
  let searchKey;
  try {
    if (!unlockedMasterKey) throw new Error('Vault is locked');
    terms = Array.isArray(params.terms) ? params.terms : [];
    params.terms = null;
    if (!vaultId || !terms.length) return { tokens: [] };
    searchKey = crypto.derive_search_key(unlockedMasterKey);
    const tokens = [];
    for (let i = 0; i < terms.length; i += 1) {
      let payload;
      let digest;
      try {
        payload = new TextEncoder().encode(vaultId + ':' + String(terms[i] || ''));
        digest = crypto.hmac_sha256(searchKey, payload);
        tokens.push(toBase64Url(digest));
      } finally {
        payload = clearBytes(crypto, payload);
        digest = clearBytes(crypto, digest);
      }
    }
    return { tokens: tokens };
  } finally {
    terms.length = 0;
    params.terms = null;
    searchKey = clearBytes(crypto, searchKey);
  }
}

function handleDecryptSearchFile(params: WorkerInput<'decryptSearchFile'>, crypto: CryptoApi) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let encryptedFileKey;
  let encryptedMetadata;
  let fileKey;
  let metadata;
  try {
    encryptedFileKey = fromBase64(params.encryptedFileKey);
    encryptedMetadata = fromBase64(params.encryptedMetadata);
    fileKey = crypto.unwrap_file_key(
      encryptedFileKey,
      unlockedMasterKey,
      aadBytes(params.fileKeyAad),
    );
    metadata = crypto.decrypt_chunk(
      encryptedMetadata,
      fileKey,
      aadBytes(params.metadataAad),
    );
		return { metadataBytes: metadata.slice() };
  } finally {
    fileKey = clearBytes(crypto, fileKey);
    encryptedFileKey = clearBytes(crypto, encryptedFileKey);
    encryptedMetadata = clearBytes(crypto, encryptedMetadata);
    metadata = clearBytes(crypto, metadata);
  }
}

function handleDecryptSearchFolder(params: WorkerInput<'decryptSearchFolder'>, crypto: CryptoApi) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let encryptedName;
  let encryptedMetadata;
  let name;
  let metadata;
  try {
    encryptedName = fromBase64(params.encryptedName);
    encryptedMetadata = fromBase64(params.encryptedMetadata);
    name = crypto.decrypt_chunk(
      encryptedName,
      unlockedMasterKey,
      aadBytes(params.nameAad),
    );
    metadata = crypto.decrypt_chunk(
      encryptedMetadata,
      unlockedMasterKey,
      aadBytes(params.metadataAad),
    );
		return { nameBytes: name.slice(), metadataBytes: metadata.slice() };
  } finally {
    encryptedName = clearBytes(crypto, encryptedName);
    encryptedMetadata = clearBytes(crypto, encryptedMetadata);
    name = clearBytes(crypto, name);
    metadata = clearBytes(crypto, metadata);
  }
}

function handleDecryptSearchThumbnail(params: WorkerInput<'decryptSearchThumbnail'>, crypto: CryptoApi) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
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
    fileKey = crypto.unwrap_file_key(
      encryptedFileKey,
      unlockedMasterKey,
      aadBytes(params.fileKeyAad),
    );
    thumbnail = crypto.decrypt_chunk(
      encryptedThumbnail,
      fileKey,
      aadBytes(params.thumbnailAad),
    );
    return { thumbnailBytes: thumbnail.slice() };
  } finally {
    encryptedFileKey = clearBytes(crypto, encryptedFileKey);
    encryptedThumbnail = clearBytes(crypto, encryptedThumbnail);
    fileKey = clearBytes(crypto, fileKey);
    thumbnail = clearBytes(crypto, thumbnail);
  }
}

const messageHandlers: WorkerHandlers = {
	createInitialVault: handleCreateInitialVault,
	prepareInitialRecovery: handlePrepareInitialRecovery,
	unlockVault: handleUnlockVault,
	lockVault: handleLockVault,
	preparePasswordReset: handlePreparePasswordReset,
	prepareShare: handlePrepareShare,
	openShareKey: handleOpenShareKey,
	prepareUpload: handlePrepareUpload,
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
	decryptSearchThumbnail: handleDecryptSearchThumbnail
};

async function handleMessage(message: WorkerRequest): Promise<WorkerResult> {
  const crypto = await ensureCrypto();
  // Map lookup loses the method/params correlation; WorkerRequest preserves it.
  const handler = messageHandlers[message.method] as WorkerHandler<VaultWorkerMethod>;
  return handler(message.params, crypto);
}

const workerScope = self as unknown as {
	addEventListener(type: 'message', listener: (event: MessageEvent<WorkerRequest>) => void): void;
	postMessage(message: WorkerResponse, transfer?: Transferable[]): void;
};

workerScope.addEventListener('message', async (event) => {
	const payload = event.data;
  try {
    const result = await handleMessage(payload);
    const transfer: Transferable[] = [];
		for (const value of [result.masterKey, result.shareSecret, result.encryptedChunk, result.encryptedThumbnail, result.chunkBytes, result.thumbnailBytes, result.metadataBytes, result.manifestBytes, result.nameBytes]) {
			if (value instanceof Uint8Array && value.buffer instanceof ArrayBuffer) transfer.push(value.buffer);
		}
		workerScope.postMessage({ id: payload.id, ok: true, result }, transfer);
  } catch (error) {
    workerScope.postMessage({
			id: payload.id,
			ok: false,
			error: error instanceof Error ? error.message : 'Vault worker failed'
		});
  }
});
