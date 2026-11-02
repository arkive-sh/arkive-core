import type { MediaDimensions } from '#lib/vault/types';

export interface ThumbnailSource {
	version: number;
	size: number;
	mime: string;
	thumbnailWidth: number;
	thumbnailHeight: number;
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
	dimensions?: MediaDimensions;
	thumbnail?: ThumbnailSource;
}

export function fileManagerEntryKey(entry: Pick<FileManagerEntry, 'id' | 'kind'>): string {
	return `${entry.kind}:${entry.id}`;
}

export function isPdfFile(entry: Pick<FileManagerEntry, 'name' | 'type'>): boolean {
	return entry.type.toLowerCase() === 'application/pdf' || entry.name.toLowerCase().endsWith('.pdf');
}

export type FilesView = 'list' | 'grid';
