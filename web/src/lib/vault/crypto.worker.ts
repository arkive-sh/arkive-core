import initArkiveCrypto, * as arkiveCrypto from '../../../../core/web/static/vendor/arkive-crypto/arkive_crypto.js';
import { deriveRecoveryProofPublicKey, signRecoveryProof } from '../recovery-proof';
import type { VaultWorkerMethod } from './types';

type CryptoApi = typeof arkiveCrypto;

interface WorkerParams {
	[key: string]: unknown;
	aad?: unknown;
	chunkBytes?: unknown;
	bytes?: unknown;
	contextId?: unknown;
	encryptedChunk?: unknown;
	encryptedFileKey?: unknown;
	encryptedFileKeyForShare?: string;
	encryptedManifest?: string;
	encryptedMasterKey?: string;
	encryptedMasterKeyRecovery?: string;
	encryptedMetadata?: string;
	encryptedName?: string;
	encryptedShareKey?: string;
	encryptedThumbnail?: unknown;
	exportKey?: unknown;
	expectedHash?: string;
	fileKey?: string;
	fileKeyAad?: string;
	hashEncoding?: string;
	manifest?: Record<string, unknown>;
	manifestAad?: string;
	masterKey?: string;
	masterKeyBytes?: ArrayBuffer;
	metadata?: Record<string, unknown>;
	metadataAad?: string;
	nameAad?: string;
	partHashes?: string[];
	recoveryKey?: string;
	resetToken?: string;
	salt?: string;
	shareFileKeyAad?: string;
	shareKeyAad?: string;
	shareSecret?: string;
	terms?: unknown;
	thumbnailAad?: string;
	thumbnailBytes?: unknown;
	totalParts?: number;
	uploadToken?: string;
	userID?: string;
	vaultId?: string;
	wrappedMasterKey?: string;
}

interface WorkerRequest {
	id: number;
	method: VaultWorkerMethod;
	params: WorkerParams;
}

interface WorkerResult {
	[key: string]: unknown;
	encryptedChunk?: unknown;
	encryptedThumbnail?: unknown;
	chunkBytes?: unknown;
	thumbnailBytes?: unknown;
}

type WorkerResponse = { id: number; ok: true; result: WorkerResult } | { id: number; ok: false; error: string };
type WorkerHandler = (params: WorkerParams, crypto: CryptoApi) => WorkerResult | Promise<WorkerResult>;
type ActiveUpload = { fileKey: Uint8Array };

let readyPromise: Promise<CryptoApi> | null = null;
let unlockedMasterKey: Uint8Array | null = null;
const activeUploads = new Map<string, ActiveUpload>();
const activeReaders = new Map<string, Uint8Array>();
const activeShareReaders = new Map<string, Uint8Array>();

function ensureCrypto(): Promise<CryptoApi> {
  if (readyPromise) {
    return readyPromise;
  }
  readyPromise = initArkiveCrypto().then(function () {
    return arkiveCrypto;
  });
  return readyPromise;
}

function encodeBase64(bytes: Uint8Array): string {
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

function encodeBase64URL(bytes: Uint8Array): string {
  return encodeBase64(bytes).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function decodeBase64(value: unknown): Uint8Array {
  const normalized = String(value || "").trim();
  if (!normalized) {
    return new Uint8Array();
  }
  const binary = atob(normalized);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

async function deriveOpaqueVaultKEK(exportKey: Uint8Array, salt: Uint8Array): Promise<Uint8Array> {
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
    rawKey.fill(0);
    rawSalt.fill(0);
    rawKey = null;
    rawSalt = null;
  }
}

function toUint8Array(value: unknown): Uint8Array {
  if (value instanceof Uint8Array) {
    return value;
  }
  if (value instanceof ArrayBuffer) {
    return new Uint8Array(value);
  }
  if (ArrayBuffer.isView(value)) {
    return new Uint8Array(value.buffer, value.byteOffset, value.byteLength);
  }
  return decodeBase64(value);
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
  if (unlockedMasterKey) {
    crypto.zeroize(unlockedMasterKey);
    unlockedMasterKey = null;
  }
  activeUploads.forEach(function(fileKey) {
    if (fileKey && fileKey.fileKey) {
      crypto.zeroize(fileKey.fileKey);
    }
  });
  activeUploads.clear();
  activeReaders.forEach(function(fileKey) {
    crypto.zeroize(fileKey);
  });
  activeReaders.clear();
  activeShareReaders.forEach(function(fileKey) {
    crypto.zeroize(fileKey);
  });
  activeShareReaders.clear();
}

function activeMasterKey(supplied?: string): { bytes: Uint8Array; temporary: boolean } {
  if (supplied) {
    return {
      bytes: decodeBase64(supplied),
      temporary: true,
    };
  }
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  return {
    bytes: unlockedMasterKey,
    temporary: false,
  };
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

async function handleUnlockVault(params: WorkerParams, crypto: CryptoApi) {
  let exportKey;
  let salt;
  let encryptedMasterKey;
  let kek;
  let masterKey;
  try {
		exportKey = toUint8Array(params.exportKey);
		salt = decodeBase64(params.salt);
    encryptedMasterKey = decodeBase64(params.encryptedMasterKey);
		kek = await deriveOpaqueVaultKEK(exportKey, salt);
    masterKey = crypto.unwrap_master_key(
      encryptedMasterKey,
      kek,
      aadBytes(params.aad),
    );
    lockVault(crypto);
    unlockedMasterKey = masterKey.slice();
    return { unlocked: true };
  } finally {
    if (exportKey) crypto.zeroize(exportKey);
    if (kek) crypto.zeroize(kek);
    if (masterKey) crypto.zeroize(masterKey);
  }
}

async function handleCreateInitialVault(params: WorkerParams, crypto: CryptoApi) {
  let exportKey;
  let salt;
  let masterKey;
  let kek;
  let encryptedMasterKey;
  try {
    exportKey = toUint8Array(params.exportKey);
    salt = crypto.generate_salt();
    masterKey = crypto.generate_master_key();
    kek = await deriveOpaqueVaultKEK(exportKey, salt);
    encryptedMasterKey = crypto.wrap_master_key(
      masterKey,
      kek,
      new TextEncoder().encode('arkive:master-key:v1'),
    );
    return {
      vaultSalt: encodeBase64(salt),
      encryptedMasterKey: encodeBase64(encryptedMasterKey),
      masterKey: masterKey.slice(),
    };
  } finally {
    if (exportKey) crypto.zeroize(exportKey);
    if (salt) crypto.zeroize(salt);
    if (masterKey) crypto.zeroize(masterKey);
    if (kek) crypto.zeroize(kek);
    if (encryptedMasterKey) crypto.zeroize(encryptedMasterKey);
    masterKey = null;
  }
}

async function handlePrepareInitialRecovery(params: WorkerParams, crypto: CryptoApi) {
  const userID = String(params.userID || '').trim();
  if (!userID) throw new Error('Recovery setup context is unavailable');

  let masterKey;
  let recoveryKey;
  let encryptedMasterKeyRecovery;
  let recoveryProofPublicKey;
  try {
    masterKey = toUint8Array(params.masterKeyBytes);
    if (!masterKey.length) throw new Error('Setup master key is unavailable');
    recoveryKey = crypto.generate_recovery_key();
    encryptedMasterKeyRecovery = crypto.wrap_master_key_with_recovery_key(masterKey, recoveryKey, userID);
    recoveryProofPublicKey = await deriveRecoveryProofPublicKey(recoveryKey, userID);
    return {
      recoveryKey: crypto.format_recovery_key(recoveryKey),
      encryptedMasterKeyRecovery: encodeBase64(encryptedMasterKeyRecovery),
      recoveryProofPublicKey: encodeBase64(recoveryProofPublicKey),
    };
  } finally {
    if (masterKey) crypto.zeroize(masterKey);
    if (recoveryKey) crypto.zeroize(recoveryKey);
    if (encryptedMasterKeyRecovery) crypto.zeroize(encryptedMasterKeyRecovery);
    if (recoveryProofPublicKey) crypto.zeroize(recoveryProofPublicKey);
    masterKey = null;
  }
}

function handleLockVault(params: WorkerParams, crypto: CryptoApi) {
  lockVault(crypto);
  return { unlocked: false };
}

async function handlePreparePasswordReset(params: WorkerParams, crypto: CryptoApi) {
  let exportKey;
  let encryptedMasterKeyRecovery;
  let recoveryKey;
  let salt;
  let masterKey;
  let recoveryProof;
  let kek;
  let wrappedMasterKey;
  try {
    exportKey = toUint8Array(params.exportKey);
    encryptedMasterKeyRecovery = decodeBase64(
      params.encryptedMasterKeyRecovery,
    );
    recoveryKey = parseRecoveryKey(crypto, params.recoveryKey);
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
    kek = await deriveOpaqueVaultKEK(exportKey, salt);
    wrappedMasterKey = crypto.wrap_master_key(
      masterKey,
      kek,
      aadBytes("arkive:master-key:v1"),
    );
    return {
      vaultSalt: encodeBase64(salt),
      encryptedMasterKey: encodeBase64(wrappedMasterKey),
      recoveryProof: encodeBase64(recoveryProof),
    };
  } finally {
    if (exportKey) crypto.zeroize(exportKey);
    if (recoveryKey) crypto.zeroize(recoveryKey);
    if (masterKey) crypto.zeroize(masterKey);
    if (kek) crypto.zeroize(kek);
  }
}

function handleGenerateFileKey(params: WorkerParams, crypto: CryptoApi) {
  let fileKey;
  try {
    fileKey = crypto.generate_file_key();
    return { fileKey: encodeBase64(fileKey) };
  } finally {
    if (fileKey) crypto.zeroize(fileKey);
  }
}

function handlePrepareShare(params: WorkerParams, crypto: CryptoApi) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let encryptedFileKey;
  let fileKey;
  let shareKey;
  let encryptedShareKey;
  let encryptedFileKeyForShare;
  try {
    encryptedFileKey = decodeBase64(params.encryptedFileKey);
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
      encryptedShareKey: encodeBase64(encryptedShareKey),
      encryptedFileKeyForShare: encodeBase64(encryptedFileKeyForShare),
      shareSecret: encodeBase64(shareKey),
      cryptoVersion: 1,
    };
  } finally {
    if (fileKey) crypto.zeroize(fileKey);
    if (shareKey) crypto.zeroize(shareKey);
  }
}

function handleOpenShareKey(params: WorkerParams, crypto: CryptoApi) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let encryptedShareKey;
  let shareKey;
  try {
    encryptedShareKey = decodeBase64(params.encryptedShareKey);
    shareKey = crypto.unwrap_file_key(
      encryptedShareKey,
      unlockedMasterKey,
      aadBytes(params.shareKeyAad),
    );
    return { shareSecret: encodeBase64(shareKey) };
  } finally {
    if (shareKey) crypto.zeroize(shareKey);
  }
}

function handlePrepareUpload(params: WorkerParams, crypto: CryptoApi) {
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
      encryptedFileKey: encodeBase64(encryptedFileKey),
      totalParts: Number(params.totalParts || 0),
      encryptionVersion: 1,
    };
  } finally {
    if (fileKey) crypto.zeroize(fileKey);
		if (encryptedFileKey) crypto.zeroize(encryptedFileKey);
  }
}

function handleEncryptUploadMetadata(params: WorkerParams, crypto: CryptoApi) {
	const uploadToken = String(params.uploadToken || '');
	const upload = activeUploads.get(uploadToken);
	if (!upload || !upload.fileKey) {
		throw new Error('Upload context is missing');
	}
	let metadata;
	let encryptedMetadata;
	try {
		metadata = new TextEncoder().encode(JSON.stringify(params.metadata || {}));
		encryptedMetadata = crypto.encrypt_chunk(metadata, upload.fileKey, aadBytes(params.metadataAad));
		return { encryptedMetadata: encodeBase64(encryptedMetadata) };
	} finally {
		if (metadata) crypto.zeroize(metadata);
		if (encryptedMetadata) crypto.zeroize(encryptedMetadata);
	}
}

function handleEncryptUploadThumbnail(params: WorkerParams, crypto: CryptoApi) {
	const uploadToken = String(params.uploadToken || '');
	const upload = activeUploads.get(uploadToken);
	if (!upload || !upload.fileKey) {
		throw new Error('Upload context is missing');
	}
	let thumbnailBytes;
	let encryptedThumbnail;
	try {
		thumbnailBytes = toUint8Array(params.thumbnailBytes);
		encryptedThumbnail = crypto.encrypt_chunk(thumbnailBytes, upload.fileKey, aadBytes(params.aad));
		return {
			encryptedThumbnail: encryptedThumbnail.slice(),
			encryptedSize: encryptedThumbnail.length
		};
	} finally {
		if (thumbnailBytes) crypto.zeroize(thumbnailBytes);
		if (encryptedThumbnail) crypto.zeroize(encryptedThumbnail);
	}
}

function handleEncryptUploadPart(params: WorkerParams, crypto: CryptoApi) {
  const uploadToken = String(params.uploadToken || "");
  const upload = activeUploads.get(uploadToken);
  if (!upload || !upload.fileKey) {
    throw new Error("Upload context is missing");
  }
  let chunkBytes;
  let encryptedChunk;
  let encryptedHash;
  try {
    chunkBytes = toUint8Array(params.chunkBytes);
    encryptedChunk = crypto.encrypt_chunk(
      chunkBytes,
      upload.fileKey,
      aadBytes(params.aad),
    );
    encryptedHash = crypto.hash_bytes_blake3(encryptedChunk);
    return {
      encryptedChunk: encryptedChunk.slice(),
      encryptedHash: encodeBase64(encryptedHash),
      encryptedSize: encryptedChunk.length,
    };
  } finally {
    if (chunkBytes) crypto.zeroize(chunkBytes);
    if (encryptedChunk) crypto.zeroize(encryptedChunk);
    if (encryptedHash) crypto.zeroize(encryptedHash);
  }
}

function handleHashUploadPayload(params: WorkerParams, crypto: CryptoApi) {
	const bytes = toUint8Array(params.bytes);
	let hash;
	try {
		hash = crypto.hash_bytes_blake3(bytes);
		return { hash: encodeBase64(hash) };
	} finally {
		crypto.zeroize(bytes);
		if (hash) crypto.zeroize(hash);
	}
}

function handleFinalizeUpload(params: WorkerParams, crypto: CryptoApi) {
  const uploadToken = String(params.uploadToken || "");
  const upload = activeUploads.get(uploadToken);
  if (!upload || !upload.fileKey) {
    throw new Error("Upload context is missing");
  }
  let manifest;
  const decoded: Uint8Array[] = [];
  let encryptedManifest;
  let combined;
  let encryptedHash;
  try {
    manifest = new TextEncoder().encode(JSON.stringify(params.manifest || {}));
    encryptedManifest = crypto.encrypt_chunk(
      manifest,
      upload.fileKey,
      aadBytes(params.manifestAad),
    );
    const partHashes = params.partHashes || [];
    let totalLength = 0;
    for (let i = 0; i < partHashes.length; i += 1) {
      const hashBytes = decodeBase64(partHashes[i]);
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
      encryptedManifest: encodeBase64(encryptedManifest),
      encryptedHash: encodeBase64(encryptedHash),
    };
  } finally {
    if (manifest) crypto.zeroize(manifest);
    if (encryptedManifest) crypto.zeroize(encryptedManifest);
    for (const hash of decoded) crypto.zeroize(hash);
    if (combined) crypto.zeroize(combined);
    if (encryptedHash) crypto.zeroize(encryptedHash);
    crypto.zeroize(upload.fileKey);
    activeUploads.delete(uploadToken);
  }
}

function handleClearUploadContext(params: WorkerParams, crypto: CryptoApi) {
  const uploadToken = String(params.uploadToken || "");
  const upload = activeUploads.get(uploadToken);
  if (upload && upload.fileKey) {
    crypto.zeroize(upload.fileKey);
    activeUploads.delete(uploadToken);
  }
  return { cleared: true };
}

function handleOpenFileContext(params: WorkerParams, crypto: CryptoApi) {
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
    encryptedFileKey = decodeBase64(params.encryptedFileKey);
    encryptedMetadata = decodeBase64(params.encryptedMetadata);
    encryptedManifest = decodeBase64(params.encryptedManifest);
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
      metadata: new TextDecoder().decode(metadataBytes),
      manifest: new TextDecoder().decode(manifestBytes),
    };
  } finally {
    if (fileKey) crypto.zeroize(fileKey);
    if (metadataBytes) crypto.zeroize(metadataBytes);
    if (manifestBytes) crypto.zeroize(manifestBytes);
  }
}

function handleCloseFileContext(params: WorkerParams, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  const fileKey = activeReaders.get(contextID);
  if (fileKey) {
    crypto.zeroize(fileKey);
    activeReaders.delete(contextID);
  }
  return { cleared: true };
}

function handleOpenPublicShareContext(params: WorkerParams, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  if (!contextID) {
    throw new Error("Missing share context");
  }
  let shareSecret;
  let encryptedFileKeyForShare;
  let encryptedMetadata;
  let encryptedManifest;
  let fileKey;
  let metadataBytes;
  let manifestBytes;
  try {
    shareSecret = decodeBase64(params.shareSecret);
    encryptedFileKeyForShare = decodeBase64(params.encryptedFileKeyForShare);
    encryptedMetadata = decodeBase64(params.encryptedMetadata);
    encryptedManifest = decodeBase64(params.encryptedManifest);
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
      metadata: new TextDecoder().decode(metadataBytes),
      manifest: new TextDecoder().decode(manifestBytes),
    };
  } finally {
    if (shareSecret) crypto.zeroize(shareSecret);
    if (fileKey) crypto.zeroize(fileKey);
    if (metadataBytes) crypto.zeroize(metadataBytes);
    if (manifestBytes) crypto.zeroize(manifestBytes);
  }
}

function handleClosePublicShareContext(params: WorkerParams, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  const fileKey = activeShareReaders.get(contextID);
  if (fileKey) {
    crypto.zeroize(fileKey);
    activeShareReaders.delete(contextID);
  }
  return { cleared: true };
}

function handleDecryptFileChunk(params: WorkerParams, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  const fileKey = activeReaders.get(contextID);
  if (!fileKey) {
    throw new Error("File context is missing");
  }
  let encryptedChunk;
  let actualHash;
  let chunkBytes;
  try {
    encryptedChunk = toUint8Array(params.encryptedChunk);
    const expectedHash = String(params.expectedHash || "");
    if (expectedHash) {
      actualHash = crypto.hash_bytes_blake3(encryptedChunk);
      if (encodeBase64(actualHash) !== expectedHash) {
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
    if (chunkBytes) crypto.zeroize(chunkBytes);
  }
}

function handleDecryptPublicShareChunk(params: WorkerParams, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  const fileKey = activeShareReaders.get(contextID);
  if (!fileKey) {
    throw new Error("Share context is missing");
  }
  let encryptedChunk;
  let actualHash;
  let chunkBytes;
  try {
    encryptedChunk = toUint8Array(params.encryptedChunk);
    const expectedHash = String(params.expectedHash || "");
    if (expectedHash) {
      actualHash = crypto.hash_bytes_blake3(encryptedChunk);
      const hashEncoding = String(params.hashEncoding || "base64").toLowerCase();
      const actual = hashEncoding === "hex"
        ? Array.from(actualHash).map(function(byte) {
          return byte.toString(16).padStart(2, "0");
        }).join("")
        : encodeBase64(actualHash);
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
    if (chunkBytes) crypto.zeroize(chunkBytes);
  }
}

function handleEncryptFileMetadataInContext(params: WorkerParams, crypto: CryptoApi) {
  const contextID = String(params.contextId || "");
  const fileKey = activeReaders.get(contextID);
  if (!fileKey) {
    throw new Error("File context is missing");
  }
  let metadata;
  let encrypted;
  try {
    metadata = new TextEncoder().encode(JSON.stringify(params.metadata || {}));
    encrypted = crypto.encrypt_chunk(
      metadata,
      fileKey,
      aadBytes(params.aad),
    );
    return { encryptedMetadata: encodeBase64(encrypted) };
  } finally {
    if (metadata) crypto.zeroize(metadata);
  }
}

function handleEncryptFileMetadata(params: WorkerParams, crypto: CryptoApi) {
  const master = activeMasterKey(params.masterKey);
  let metadata;
  let encrypted;
  try {
    metadata = new TextEncoder().encode(JSON.stringify(params.metadata || {}));
    encrypted = crypto.encrypt_chunk(
      metadata,
      master.bytes,
      aadBytes(params.aad),
    );
    return { encryptedMetadata: encodeBase64(encrypted) };
  } finally {
    if (master.temporary) crypto.zeroize(master.bytes);
    if (metadata) crypto.zeroize(metadata);
  }
}

function handleDecryptFileMetadata(params: WorkerParams, crypto: CryptoApi) {
  const master = activeMasterKey(params.masterKey);
  let encryptedMetadata;
  let metadata;
  try {
    encryptedMetadata = decodeBase64(params.encryptedMetadata);
    metadata = crypto.decrypt_chunk(
      encryptedMetadata,
      master.bytes,
      aadBytes(params.aad),
    );
    return { metadata: JSON.parse(new TextDecoder().decode(metadata)) };
  } finally {
    if (master.temporary) crypto.zeroize(master.bytes);
    if (metadata) crypto.zeroize(metadata);
  }
}

function handleEncryptFileKey(params: WorkerParams, crypto: CryptoApi) {
  const master = activeMasterKey(params.masterKey);
  let fileKey;
  let encryptedFileKey;
  try {
    fileKey = decodeBase64(params.fileKey);
    encryptedFileKey = crypto.wrap_file_key(
      fileKey,
      master.bytes,
      aadBytes(params.aad),
    );
    return { encryptedFileKey: encodeBase64(encryptedFileKey) };
  } finally {
    if (master.temporary) crypto.zeroize(master.bytes);
    if (fileKey) crypto.zeroize(fileKey);
  }
}

function handleEncryptChunk(params: WorkerParams, crypto: CryptoApi) {
  let fileKey;
  let chunkBytes;
  let encryptedChunk;
  try {
    fileKey = decodeBase64(params.fileKey);
    chunkBytes = decodeBase64(params.chunkBytes);
    encryptedChunk = crypto.encrypt_chunk(
      chunkBytes,
      fileKey,
      aadBytes(params.aad),
    );
    return { encryptedChunk: encodeBase64(encryptedChunk) };
  } finally {
    if (fileKey) crypto.zeroize(fileKey);
    if (chunkBytes) crypto.zeroize(chunkBytes);
  }
}

function handleDecryptChunk(params: WorkerParams, crypto: CryptoApi) {
  let fileKey;
  let encryptedChunk;
  let chunkBytes;
  try {
    fileKey = decodeBase64(params.fileKey);
    encryptedChunk = decodeBase64(params.encryptedChunk);
    chunkBytes = crypto.decrypt_chunk(
      encryptedChunk,
      fileKey,
      aadBytes(params.aad),
    );
    return { chunkBytes: encodeBase64(chunkBytes) };
  } finally {
    if (fileKey) crypto.zeroize(fileKey);
    if (chunkBytes) crypto.zeroize(chunkBytes);
  }
}

function handleCreateSearchTokens(params: WorkerParams, crypto: CryptoApi) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  const vaultId = String(params.vaultId || "");
  const terms = Array.isArray(params.terms) ? params.terms : [];
  if (!vaultId || !terms.length) {
    return { tokens: [] };
  }
  let searchKey;
  try {
    searchKey = crypto.derive_search_key(unlockedMasterKey);
    const tokens = [];
    for (let i = 0; i < terms.length; i += 1) {
      const payload = new TextEncoder().encode(
        vaultId + ":" + String(terms[i] || ""),
      );
      const digest = crypto.hmac_sha256(searchKey, payload);
      tokens.push(encodeBase64URL(digest));
    }
    return { tokens: tokens };
  } finally {
    if (searchKey) crypto.zeroize(searchKey);
  }
}

function handleDecryptSearchFile(params: WorkerParams, crypto: CryptoApi) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let encryptedFileKey;
  let encryptedMetadata;
  let fileKey;
  let metadata;
  try {
    encryptedFileKey = decodeBase64(params.encryptedFileKey);
    encryptedMetadata = decodeBase64(params.encryptedMetadata);
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
    return { metadata: JSON.parse(new TextDecoder().decode(metadata)) };
  } finally {
    if (fileKey) crypto.zeroize(fileKey);
    if (metadata) crypto.zeroize(metadata);
  }
}

function handleDecryptSearchFolder(params: WorkerParams, crypto: CryptoApi) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let encryptedName;
  let encryptedMetadata;
  let name;
  let metadata;
  try {
    encryptedName = decodeBase64(params.encryptedName);
    encryptedMetadata = decodeBase64(params.encryptedMetadata);
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
    return {
      name: JSON.parse(new TextDecoder().decode(name)),
      metadata: JSON.parse(new TextDecoder().decode(metadata)),
    };
  } finally {
    if (name) crypto.zeroize(name);
    if (metadata) crypto.zeroize(metadata);
  }
}

function handleDecryptSearchThumbnail(params: WorkerParams, crypto: CryptoApi) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let encryptedFileKey;
  let encryptedThumbnail;
  let fileKey;
  let thumbnail;
  try {
    encryptedFileKey = decodeBase64(params.encryptedFileKey);
    encryptedThumbnail = toUint8Array(params.encryptedThumbnail);
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
    if (fileKey) crypto.zeroize(fileKey);
    if (thumbnail) crypto.zeroize(thumbnail);
  }
}

const messageHandlers: Record<VaultWorkerMethod, WorkerHandler> = {
	createInitialVault: handleCreateInitialVault,
	prepareInitialRecovery: handlePrepareInitialRecovery,
	unlockVault: handleUnlockVault,
	lockVault: handleLockVault,
	preparePasswordReset: handlePreparePasswordReset,
	generateFileKey: handleGenerateFileKey,
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
	encryptFileKey: handleEncryptFileKey,
	encryptChunk: handleEncryptChunk,
	decryptChunk: handleDecryptChunk,
	createSearchTokens: handleCreateSearchTokens,
	decryptSearchFile: handleDecryptSearchFile,
	decryptSearchFolder: handleDecryptSearchFolder,
	decryptSearchThumbnail: handleDecryptSearchThumbnail
};

async function handleMessage(message: WorkerRequest): Promise<WorkerResult> {
  const crypto = await ensureCrypto();
  return messageHandlers[message.method](message.params, crypto);
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
		for (const value of [result.masterKey, result.encryptedChunk, result.encryptedThumbnail, result.chunkBytes, result.thumbnailBytes]) {
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
