// AAD is public deterministic context that binds each entry to its parent folder.
export const VAULT_MASTER_KEY_AAD = 'arkive:master-key:v1';
export const FILE_METADATA_AAD = 'arkive:file-metadata:v1';

export function recoveryMasterKeyAAD(userID: string): string {
	return `arkive:master-key:recovery:v1:${userID}`;
}

export function fileKeyAAD(vaultId: string, fileId: string, parentFolderId: string | null): string {
	return `arkive:file-key:v1:${vaultId}:${fileId}:${parentFolderId ?? 'root'}`;
}

export function fileMetadataAAD(vaultId: string, fileId: string): string {
	return `${FILE_METADATA_AAD}:${vaultId}:${fileId}`;
}

export function fileManifestAAD(vaultId: string, fileId: string): string {
	return `arkive:file-manifest:v1:${vaultId}:${fileId}`;
}

export function fileThumbnailAAD(vaultId: string, fileId: string): string {
	return `arkive:file-thumbnail:v1:${vaultId}:${fileId}`;
}

export function fileChunkAAD(
	vaultId: string,
	fileId: string,
	chunkNumber: number,
	chunkSize: number,
	totalChunks: number,
): string {
	return `arkive:file-chunk:v1:${vaultId}:${fileId}:${chunkNumber}:${chunkSize}:${totalChunks}`;
}

export function shareKeyAAD(token: string): string {
	return `arkive:share-key:v1:${token}`;
}

export function shareFileKeyAAD(fileId: string, token: string): string {
	return `arkive:share-file-key:v1:${fileId}:${token}`;
}

export function shareFolderNameAAD(folderId: string, token: string): string {
	return `arkive:share-folder-name:v1:${folderId}:${token}`;
}

export function shareFolderMetadataAAD(folderId: string, token: string): string {
	return `arkive:share-folder-metadata:v1:${folderId}:${token}`;
}

export function folderNameAAD(
	vaultId: string,
	folderId: string,
	parentFolderId: string | null,
): string {
	return `arkive:folder-name:v1:${vaultId}:${folderId}:${parentFolderId ?? 'root'}`;
}

export function folderMetadataAAD(
	vaultId: string,
	folderId: string,
	parentFolderId: string | null,
): string {
	return `arkive:folder-metadata:v1:${vaultId}:${folderId}:${parentFolderId ?? 'root'}`;
}
