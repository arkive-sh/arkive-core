export type VaultBytes = Uint8Array | ArrayBuffer;
export type VaultAAD = string | Uint8Array;
export type VaultMetadata = Record<string, unknown>;

export interface SearchPreview {
	has_thumbnail?: boolean;
	thumbnail_version?: number;
	thumbnail_size?: number;
	thumbnail_mime?: string;
	thumbnail_width?: number;
	thumbnail_height?: number;
}

export interface EncryptedSearchFile {
	id: string;
	folderId: string | null;
	vaultId: string;
	encryptedMetadata: string;
	encryptedFileKey: string;
	score: number;
	url: string;
}

export interface EncryptedSearchFolder {
	id: string;
	vaultId: string;
	encryptedName: string;
	encryptedMetadata: string;
	score: number;
	url: string;
}

export interface SearchTokenEntry {
	token: string;
	field: 'name';
	weight: 10;
}

export interface VaultState {
	unlocked: boolean;
}

export interface ShareFileRecord {
	vaultId: string;
	fileId: string;
	encryptedFileKey: string;
}

export interface FileCryptoRecord extends ShareFileRecord {
	encryptedMetadata: string;
	encryptedManifest: string;
}

export interface PublicShareCryptoRecord {
	encryptedFileKeyForShare: string;
	encryptedMetadata: string;
	encryptedManifest: string;
	shareFileKeyAad: string;
	metadataAad: string;
	manifestAad: string;
}

/** Typed RPC boundary between the Svelte app and the crypto worker. */
export interface VaultWorkerCalls {
	createInitialVault: {
		params: { exportKey: ArrayBuffer };
		result: { vaultSalt: string; encryptedMasterKey: string; masterKey: Uint8Array<ArrayBuffer> };
	};
	prepareInitialRecovery: {
		params: { masterKeyBytes: ArrayBuffer; userID: string };
		result: { recoveryKey: string; encryptedMasterKeyRecovery: string; recoveryProofPublicKey: string };
	};
	unlockVault: {
		params: { exportKey: ArrayBuffer; salt: string; encryptedMasterKey: string; aad: string };
		result: VaultState;
	};
	lockVault: { params: Record<string, never>; result: VaultState };
	preparePasswordReset: {
		params: { encryptedMasterKeyRecovery: string; recoveryKey: string; exportKey: ArrayBuffer; userID: string; resetToken: string };
		result: { vaultSalt: string; encryptedMasterKey: string; recoveryProof: string };
	};
	generateFileKey: { params: Record<string, never>; result: { fileKey: string } };
	prepareShare: {
		params: { vaultId: string; fileId: string; token: string; encryptedFileKey: string; fileKeyAad: string; shareKeyAad: string; shareFileKeyAad: string };
		result: { encryptedShareKey: string; encryptedFileKeyForShare: string; shareSecret: string; cryptoVersion: number };
	};
	openShareKey: {
		params: { encryptedShareKey: string; shareKeyAad: string };
		result: { shareSecret: string };
	};
	prepareUpload: {
		params: { uploadToken: string; totalParts: number; fileKeyAad: string };
		result: { encryptedFileKey: string; totalParts: number; encryptionVersion: number };
	};
	encryptUploadMetadata: {
		params: { uploadToken: string; metadata: VaultMetadata; metadataAad: string };
		result: { encryptedMetadata: string };
	};
	encryptUploadThumbnail: {
		params: { uploadToken: string; thumbnailBytes: VaultBytes; aad: VaultAAD };
		result: { encryptedThumbnail: Uint8Array; encryptedSize: number };
	};
	encryptUploadPart: {
		params: { uploadToken: string; chunkBytes: VaultBytes; aad: VaultAAD };
		result: { encryptedChunk: Uint8Array; encryptedHash: string; encryptedSize: number };
	};
	hashUploadPayload: { params: { bytes: VaultBytes }; result: { hash: string } };
	finalizeUpload: {
		params: { uploadToken: string; manifest: VaultMetadata; partHashes: string[]; manifestAad: string };
		result: { encryptedManifest: string; encryptedHash: string };
	};
	clearUploadContext: { params: { uploadToken: string }; result: { cleared: boolean } };
	openFileContext: {
		params: { contextId: string; encryptedFileKey: string; encryptedMetadata: string; encryptedManifest: string; fileKeyAad: string; metadataAad: string; manifestAad: string };
		result: { metadata: string; manifest: string };
	};
	closeFileContext: { params: { contextId: string }; result: { cleared: boolean } };
	openPublicShareContext: {
		params: { contextId: string; shareSecret: string } & PublicShareCryptoRecord;
		result: { metadata: string; manifest: string };
	};
	closePublicShareContext: { params: { contextId: string }; result: { cleared: boolean } };
	decryptPublicShareChunk: {
		params: { contextId: string; encryptedChunk: VaultBytes; aad: VaultAAD; expectedHash: string; hashEncoding: 'base64' | 'hex' };
		result: { chunkBytes: Uint8Array };
	};
	decryptFileChunk: {
		params: { contextId: string; encryptedChunk: VaultBytes; aad: VaultAAD; expectedHash: string };
		result: { chunkBytes: Uint8Array };
	};
	encryptFileMetadataInContext: {
		params: { contextId: string; metadata: VaultMetadata; aad: string };
		result: { encryptedMetadata: string };
	};
	encryptFileMetadata: {
		params: { metadata: VaultMetadata; masterKey: string; aad: string };
		result: { encryptedMetadata: string };
	};
	encryptFolderMetadata: VaultWorkerCalls['encryptFileMetadata'];
	decryptFileMetadata: {
		params: { encryptedMetadata: string; masterKey: string; aad: string };
		result: { metadata: VaultMetadata };
	};
	decryptFolderMetadata: VaultWorkerCalls['decryptFileMetadata'];
	encryptFileKey: { params: { fileKey: string; masterKey: string; aad: VaultAAD }; result: { encryptedFileKey: string } };
	encryptChunk: { params: { chunkBytes: string; fileKey: string; aad: VaultAAD }; result: { encryptedChunk: string } };
	decryptChunk: { params: { encryptedChunk: string; fileKey: string; aad: VaultAAD }; result: { chunkBytes: string } };
	createSearchTokens: { params: { vaultId: string; terms: string[] }; result: { tokens: string[] } };
	decryptSearchFile: {
		params: { fileId: string; vaultId: string; encryptedMetadata: string; encryptedFileKey: string; fileKeyAad: string; metadataAad: string };
		result: { metadata: VaultMetadata & { name?: string; mime?: string; preview?: SearchPreview } };
	};
	decryptSearchFolder: {
		params: { encryptedName: string; encryptedMetadata: string; nameAad: string; metadataAad: string };
		result: { name: VaultMetadata & { name?: string }; metadata: VaultMetadata & { name?: string } };
	};
	decryptSearchThumbnail: {
		params: { encryptedFileKey: string; encryptedThumbnail: VaultBytes; fileKeyAad: string; thumbnailAad: string };
		result: { thumbnailBytes: Uint8Array };
	};
}

export type VaultWorkerMethod = keyof VaultWorkerCalls;
