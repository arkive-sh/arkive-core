export interface ThumbnailSource {
	version: number;
	size: number;
	mime: string;
	vaultId: string;
	encryptedMetadata: string;
	encryptedFileKey: string;
}

export interface FileManagerEntry {
	id: string;
	kind: 'folder' | 'file';
	name: string;
	type: string;
	size: number | null;
	updatedAt: string;
	thumbnail?: ThumbnailSource;
}

export type FilesView = 'list' | 'grid';
