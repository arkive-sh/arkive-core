export interface ThumbnailSource {
	version: number;
	size: number;
	mime: string;
	width: number;
	height: number;
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

export function fileManagerEntryKey(entry: Pick<FileManagerEntry, 'id' | 'kind'>): string {
	return `${entry.kind}:${entry.id}`;
}

export type FilesView = 'list' | 'grid';
