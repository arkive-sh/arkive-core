import initArkiveCrypto, * as arkiveCrypto from "../vendor/arkive-crypto/arkive_crypto.js";
import { signRecoveryProof } from "../lib/recovery_proof.js";

let readyPromise = null;
let unlockedMasterKey = null;
let activeUploads = new Map();
let activeReaders = new Map();
let activeShareReaders = new Map();

function ensureCrypto() {
  if (readyPromise) {
    return readyPromise;
  }
  readyPromise = initArkiveCrypto({
    module_or_path: "/static/vendor/arkive-crypto/arkive_crypto_bg.wasm",
  }).then(function () {
    return arkiveCrypto;
  });
  return readyPromise;
}

function encodeBase64(bytes) {
  let binary = "";
  for (let i = 0; i < bytes.length; i++) {
    binary += String.fromCharCode(bytes[i]);
  }
  return btoa(binary);
}

function encodeBase64URL(bytes) {
  return encodeBase64(bytes).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/g, "");
}

function decodeBase64(value) {
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

function toUint8Array(value) {
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

function aadBytes(value) {
  if (!value) {
    return new Uint8Array();
  }
  if (value instanceof Uint8Array) {
    return value;
  }
  return new TextEncoder().encode(String(value));
}

function lockVault(crypto) {
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

function activeMasterKey(supplied) {
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

function parseRecoveryKey(crypto, value) {
  try {
    return crypto.parse_recovery_key(String(value || "").trim());
  } catch (_) {
    throw new Error("Invalid recovery key");
  }
}

function unwrapRecoveryMasterKey(
  crypto,
  encryptedMasterKey,
  recoveryKey,
  userID,
) {
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

function handleUnlockVault(params, crypto) {
  let salt;
  let encryptedMasterKey;
  let kek;
  let masterKey;
  try {
    salt = decodeBase64(params.salt);
    encryptedMasterKey = decodeBase64(params.encryptedMasterKey);
    kek = crypto.derive_password_kek(
      String(params.password || ""),
      salt,
    );
    masterKey = crypto.unwrap_master_key(
      encryptedMasterKey,
      kek,
      aadBytes(params.aad),
    );
    lockVault(crypto);
    unlockedMasterKey = masterKey.slice();
    return { unlocked: true };
  } finally {
    if (kek) crypto.zeroize(kek);
    if (masterKey) crypto.zeroize(masterKey);
  }
}

function handleLockVault(params, crypto) {
  lockVault(crypto);
  return { unlocked: false };
}

function handleCreateSessionUnlock(params, crypto) {
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let sessionUnwrapKey;
  let wrappedMasterKey;
  try {
    sessionUnwrapKey = toUint8Array(params.sessionUnwrapKey);
    wrappedMasterKey = crypto.wrap_master_key(
      unlockedMasterKey,
      sessionUnwrapKey,
      aadBytes(params.aad),
    );
    return { wrappedMasterKey: encodeBase64(wrappedMasterKey) };
  } finally {
    if (sessionUnwrapKey) crypto.zeroize(sessionUnwrapKey);
  }
}

function handleRestoreSessionUnlock(params, crypto) {
  let sessionUnwrapKey;
  let wrappedMasterKey;
  let masterKey;
  try {
    sessionUnwrapKey = toUint8Array(params.sessionUnwrapKey);
    wrappedMasterKey = decodeBase64(params.wrappedMasterKey);
    masterKey = crypto.unwrap_master_key(
      wrappedMasterKey,
      sessionUnwrapKey,
      aadBytes(params.aad),
    );
    lockVault(crypto);
    unlockedMasterKey = masterKey.slice();
    return { unlocked: true };
  } finally {
    if (sessionUnwrapKey) crypto.zeroize(sessionUnwrapKey);
    if (masterKey) crypto.zeroize(masterKey);
  }
}

async function handlePreparePasswordReset(params, crypto) {
  let encryptedMasterKeyRecovery;
  let recoveryKey;
  let salt;
  let masterKey;
  let recoveryProof;
  let kek;
  let wrappedMasterKey;
  try {
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
    // Keep vault wrapping consistent with setup/login: password-derived KEK + AAD.
    kek = crypto.derive_password_kek(
      String(params.newPassword || ""),
      salt,
    );
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
    if (recoveryKey) crypto.zeroize(recoveryKey);
    if (masterKey) crypto.zeroize(masterKey);
    if (kek) crypto.zeroize(kek);
  }
}

function handleGenerateFileKey(params, crypto) {
  let fileKey;
  try {
    fileKey = crypto.generate_file_key();
    return { fileKey: encodeBase64(fileKey) };
  } finally {
    if (fileKey) crypto.zeroize(fileKey);
  }
}

function handlePrepareShare(params, crypto) {
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

function handleOpenShareKey(params, crypto) {
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

function handlePrepareUpload(params, crypto) {
  const uploadToken = String(params.uploadToken || "");
  if (!uploadToken) {
    throw new Error("Missing upload token");
  }
  if (!unlockedMasterKey) {
    throw new Error("Vault is locked");
  }
  let metadata;
  let fileKey;
  let encryptedMetadata;
  let encryptedFileKey;
  try {
    metadata = new TextEncoder().encode(JSON.stringify(params.metadata || {}));
    fileKey = crypto.generate_file_key();
    encryptedMetadata = crypto.encrypt_chunk(
      metadata,
      fileKey,
      aadBytes(params.metadataAad),
    );
    encryptedFileKey = crypto.wrap_file_key(
      fileKey,
      unlockedMasterKey,
      aadBytes(params.fileKeyAad),
    );
    activeUploads.set(uploadToken, { fileKey: fileKey.slice() });
    return {
      encryptedMetadata: encodeBase64(encryptedMetadata),
      encryptedFileKey: encodeBase64(encryptedFileKey),
      totalParts: Number(params.totalParts || 0),
      encryptionVersion: 1,
    };
  } finally {
    if (metadata) crypto.zeroize(metadata);
    if (fileKey) crypto.zeroize(fileKey);
  }
}

function handleEncryptUploadPart(params, crypto) {
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
  }
}

function handleFinalizeUpload(params, crypto) {
  const uploadToken = String(params.uploadToken || "");
  const upload = activeUploads.get(uploadToken);
  if (!upload || !upload.fileKey) {
    throw new Error("Upload context is missing");
  }
  let manifest;
  const decoded = [];
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
    crypto.zeroize(upload.fileKey);
    activeUploads.delete(uploadToken);
  }
}

function handleClearUploadContext(params, crypto) {
  const uploadToken = String(params.uploadToken || "");
  const upload = activeUploads.get(uploadToken);
  if (upload && upload.fileKey) {
    crypto.zeroize(upload.fileKey);
    activeUploads.delete(uploadToken);
  }
  return { cleared: true };
}

function handleOpenFileContext(params, crypto) {
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

function handleCloseFileContext(params, crypto) {
  const contextID = String(params.contextId || "");
  const fileKey = activeReaders.get(contextID);
  if (fileKey) {
    crypto.zeroize(fileKey);
    activeReaders.delete(contextID);
  }
  return { cleared: true };
}

function handleOpenPublicShareContext(params, crypto) {
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

function handleClosePublicShareContext(params, crypto) {
  const contextID = String(params.contextId || "");
  const fileKey = activeShareReaders.get(contextID);
  if (fileKey) {
    crypto.zeroize(fileKey);
    activeShareReaders.delete(contextID);
  }
  return { cleared: true };
}

function handleDecryptFileChunk(params, crypto) {
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

function handleDecryptPublicShareChunk(params, crypto) {
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

function handleEncryptFileMetadataInContext(params, crypto) {
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

function handleEncryptFileMetadata(params, crypto) {
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

function handleDecryptFileMetadata(params, crypto) {
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

function handleEncryptFileKey(params, crypto) {
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

function handleEncryptChunk(params, crypto) {
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

function handleDecryptChunk(params, crypto) {
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

function handleCreateSearchTokens(params, crypto) {
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

function handleDecryptSearchFile(params, crypto) {
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

function handleDecryptSearchFolder(params, crypto) {
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

function handleDecryptSearchThumbnail(params, crypto) {
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

const messageHandlers = new Map([
  ["unlockVault", handleUnlockVault],
  ["lockVault", handleLockVault],
  ["createSessionUnlock", handleCreateSessionUnlock],
  ["restoreSessionUnlock", handleRestoreSessionUnlock],
  ["preparePasswordReset", handlePreparePasswordReset],
  ["generateFileKey", handleGenerateFileKey],
  ["prepareShare", handlePrepareShare],
  ["openShareKey", handleOpenShareKey],
  ["prepareUpload", handlePrepareUpload],
  ["encryptUploadPart", handleEncryptUploadPart],
  ["finalizeUpload", handleFinalizeUpload],
  ["clearUploadContext", handleClearUploadContext],
  ["openFileContext", handleOpenFileContext],
  ["closeFileContext", handleCloseFileContext],
  ["openPublicShareContext", handleOpenPublicShareContext],
  ["closePublicShareContext", handleClosePublicShareContext],
  ["decryptFileChunk", handleDecryptFileChunk],
  ["decryptPublicShareChunk", handleDecryptPublicShareChunk],
  ["encryptFileMetadataInContext", handleEncryptFileMetadataInContext],
  ["encryptFileMetadata", handleEncryptFileMetadata],
  ["encryptFolderMetadata", handleEncryptFileMetadata],
  ["decryptFileMetadata", handleDecryptFileMetadata],
  ["decryptFolderMetadata", handleDecryptFileMetadata],
  ["encryptFileKey", handleEncryptFileKey],
  ["encryptChunk", handleEncryptChunk],
  ["decryptChunk", handleDecryptChunk],
  ["createSearchTokens", handleCreateSearchTokens],
  ["decryptSearchFile", handleDecryptSearchFile],
  ["decryptSearchFolder", handleDecryptSearchFolder],
  ["decryptSearchThumbnail", handleDecryptSearchThumbnail]
]);

async function handleMessage(message) {
  const crypto = await ensureCrypto();
  const handler = messageHandlers.get(message.method);
  if (!handler) {
    throw new Error("Unsupported vault method");
  }
  return handler(message.params, crypto);
}

self.addEventListener("message", async function (event) {
  const payload = event.data || {};
  try {
    const result = await handleMessage(payload);
    const transfer = [];
    if (result && result.encryptedChunk instanceof Uint8Array) {
      transfer.push(result.encryptedChunk.buffer);
    }
    if (result && result.chunkBytes instanceof Uint8Array) {
      transfer.push(result.chunkBytes.buffer);
    }
    if (result && result.thumbnailBytes instanceof Uint8Array) {
      transfer.push(result.thumbnailBytes.buffer);
    }
    self.postMessage({
      id: payload.id,
      ok: true,
      result: result,
    }, transfer);
  } catch (error) {
    self.postMessage({
      id: payload.id,
      ok: false,
      error: error && error.message ? error.message : "Vault worker failed",
    });
  }
});
