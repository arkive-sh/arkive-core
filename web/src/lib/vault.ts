export interface SearchPreview {
	has_thumbnail?: boolean;
	thumbnail_version?: number;
	thumbnail_size?: number;
	thumbnail_mime?: string;
}

export interface EncryptedSearchFile {
	id: string;
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

export interface SearchVault {
	createSearchTokens(query: string, vaultId: string): Promise<string[]>;
	decryptSearchFile(file: EncryptedSearchFile): Promise<{ metadata?: { name?: string; mime?: string; preview?: SearchPreview } }>;
	decryptSearchFolder(folder: EncryptedSearchFolder): Promise<{ name?: { name?: string }; metadata?: { name?: string } }>;
	decryptSearchThumbnail(file: EncryptedSearchFile, encryptedThumbnail: Uint8Array): Promise<{ thumbnailBytes: Uint8Array }>;
}

interface VaultModule extends SearchVault {
	unlockVault(password: string, salt: string, encryptedMasterKey: string): Promise<unknown>;
	persistSessionUnlock(): Promise<unknown>;
	clearSessionUnlock(): void;
}

// shortcut: reuse the Go UI's vault runtime until its crypto worker is migrated too.
async function loadVault(): Promise<VaultModule> {
	return (await import('../../../core/web/static/features/vault.js')) as unknown as VaultModule;
}

export async function loadSearchVault(): Promise<SearchVault> {
	return loadVault();
}

export async function unlockForLogin(password: string, salt: string, encryptedMasterKey: string): Promise<void> {
	const vault = await loadVault();
	await vault.unlockVault(password, salt, encryptedMasterKey);
	await vault.persistSessionUnlock();
}

export async function clearSessionUnlock(): Promise<void> {
	const vault = await loadVault();
	vault.clearSessionUnlock();
}
