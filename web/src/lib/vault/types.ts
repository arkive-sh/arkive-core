export type VaultBytes = Uint8Array | ArrayBuffer;
export type VaultAAD = string | Uint8Array;
export type VaultMetadata = Record<string, unknown>;

export interface MediaDimensions {
	width: number;
	height: number;
}

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
	parentFolderId: string | null;
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
	folderId: string | null;
	encryptedFileKey: string;
}

export interface FileCryptoRecord extends ShareFileRecord {
	encryptedMetadata: string;
	encryptedManifest: string;
}

export interface PublicShareCryptoRecord {
	// Derive AAD client-side from the public identifiers rather than accepting it from the API.
	token: string;
	fileId: string;
	vaultId: string;
	encryptedFileKeyForShare: string;
	encryptedMetadata: string;
	encryptedManifest: string;
}

/** Typed RPC boundary between the Svelte app and the crypto worker. */
export interface VaultWorkerCalls {
	createInitialVault: {
		params: { exportKey: ArrayBuffer | null };
		result: { vaultSalt: string; encryptedMasterKey: string; masterKey: Uint8Array<ArrayBuffer> };
	};
	prepareInitialRecovery: {
		params: { masterKeyBytes: ArrayBuffer | null; userID: string };
		result: {
			recoveryKey: string;
			encryptedMasterKeyRecovery: string;
			recoveryProofPublicKey: string;
		};
	};
	unlockVault: {
		params: {
			exportKey: ArrayBuffer | null;
			salt: string;
			encryptedMasterKey: string;
			aad: string;
		};
		result: VaultState;
	};
	lockVault: { params: Record<string, never>; result: VaultState };
	preparePasswordReset: {
		params: {
			encryptedMasterKeyRecovery: string;
			recoveryKey: VaultBytes | null;
			exportKey: ArrayBuffer | null;
			userID: string;
			resetToken: string;
		};
		result: { vaultSalt: string; encryptedMasterKey: string; recoveryProof: string };
	};
	prepareShare: {
		params: {
			vaultId: string;
			fileId: string;
			token: string;
			encryptedFileKey: string;
			fileKeyAad: string;
			shareKeyAad: string;
			shareFileKeyAad: string;
		};
		result: {
			encryptedShareKey: string;
			encryptedFileKeyForShare: string;
			shareSecret: Uint8Array<ArrayBuffer>;
			cryptoVersion: number;
		};
	};
	openShareKey: {
		params: { encryptedShareKey: string; shareKeyAad: string };
		result: { shareSecret: Uint8Array<ArrayBuffer> };
	};
	prepareUpload: {
		params: { uploadToken: string; totalParts: number; fileKeyAad: string };
		result: { encryptedFileKey: string; totalParts: number; encryptionVersion: number };
	};
	rewrapFileKey: {
		params: { encryptedFileKey: string; oldAad: string; newAad: string };
		result: { encryptedFileKey: string };
	};
	encryptUploadMetadata: {
		params: { uploadToken: string; metadataBytes: ArrayBuffer | null; metadataAad: string };
		result: { encryptedMetadata: string };
	};
	encryptUploadThumbnail: {
		params: { uploadToken: string; thumbnailBytes: VaultBytes | null; aad: VaultAAD };
		result: { encryptedThumbnail: Uint8Array; encryptedSize: number };
	};
	encryptUploadPart: {
		params: { uploadToken: string; chunkBytes: VaultBytes | null; aad: VaultAAD };
		result: { encryptedChunk: Uint8Array; encryptedHash: string; encryptedSize: number };
	};
	hashUploadPayload: { params: { bytes: VaultBytes | null }; result: { hash: string } };
	finalizeUpload: {
		params: {
			uploadToken: string;
			manifestBytes: ArrayBuffer | null;
			partHashes: string[];
			manifestAad: string;
		};
		result: { encryptedManifest: string; encryptedHash: string };
	};
	clearUploadContext: { params: { uploadToken: string }; result: { cleared: boolean } };
	openFileContext: {
		params: {
			contextId: string;
			encryptedFileKey: string;
			encryptedMetadata: string;
			encryptedManifest: string;
			fileKeyAad: string;
			metadataAad: string;
			manifestAad: string;
		};
		result: { metadataBytes: Uint8Array; manifestBytes: Uint8Array };
	};
	closeFileContext: { params: { contextId: string }; result: { cleared: boolean } };
	openPublicShareContext: {
		params: {
			contextId: string;
			shareSecret: VaultBytes | null;
			encryptedFileKeyForShare: string;
			encryptedMetadata: string;
			encryptedManifest: string;
			shareFileKeyAad: string;
			metadataAad: string;
			manifestAad: string;
		};
		result: { metadataBytes: Uint8Array; manifestBytes: Uint8Array };
	};
	closePublicShareContext: { params: { contextId: string }; result: { cleared: boolean } };
	decryptPublicShareChunk: {
		params: {
			contextId: string;
			encryptedChunk: VaultBytes | null;
			aad: VaultAAD;
			expectedHash: string;
			hashEncoding: 'base64' | 'hex';
		};
		result: { chunkBytes: Uint8Array };
	};
	decryptFileChunk: {
		params: {
			contextId: string;
			encryptedChunk: VaultBytes | null;
			aad: VaultAAD;
			expectedHash: string;
		};
		result: { chunkBytes: Uint8Array };
	};
	encryptFileMetadataInContext: {
		params: { contextId: string; metadataBytes: ArrayBuffer | null; aad: string };
		result: { encryptedMetadata: string };
	};
	encryptFileMetadata: {
		params: { metadataBytes: ArrayBuffer | null; aad: string };
		result: { encryptedMetadata: string };
	};
	encryptFolderMetadata: VaultWorkerCalls['encryptFileMetadata'];
	decryptFileMetadata: {
		params: { encryptedMetadata: string; aad: string };
		result: { metadataBytes: Uint8Array };
	};
	decryptFolderMetadata: VaultWorkerCalls['decryptFileMetadata'];
	createSearchTokens: {
		params: { vaultId: string; terms: string[] | null };
		result: { tokens: string[] };
	};
	decryptSearchFile: {
		params: {
			fileId: string;
			vaultId: string;
			encryptedMetadata: string;
			encryptedFileKey: string;
			fileKeyAad: string;
			metadataAad: string;
		};
		result: { metadataBytes: Uint8Array };
	};
	decryptSearchFolder: {
		params: {
			encryptedName: string;
			encryptedMetadata: string;
			nameAad: string;
			metadataAad: string;
		};
		result: { nameBytes: Uint8Array; metadataBytes: Uint8Array };
	};
	decryptSearchThumbnail: {
		params: {
			encryptedFileKey: string;
			encryptedThumbnail: VaultBytes | null;
			fileKeyAad: string;
			thumbnailAad: string;
		};
		result: { thumbnailBytes: Uint8Array };
	};
}

export type VaultWorkerMethod = keyof VaultWorkerCalls;
