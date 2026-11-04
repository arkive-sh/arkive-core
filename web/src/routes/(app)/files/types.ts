import type { MediaDimensions } from '#lib/vault/types.ts';

export const NEW_FOLDER_DRAFT_ID = '__new-folder-draft__';

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
	selectable?: boolean;
	isTrashContainer?: boolean;
	dimensions?: MediaDimensions;
	thumbnail?: ThumbnailSource;
}

export function fileManagerEntryKey(entry: Pick<FileManagerEntry, 'id' | 'kind'>): string {
	return `${entry.kind}:${entry.id}`;
}

export function isPdfFile(entry: Pick<FileManagerEntry, 'name' | 'type'>): boolean {
	return (
		entry.type.toLowerCase() === 'application/pdf' || entry.name.toLowerCase().endsWith('.pdf')
	);
}

const TEXT_EXTENSIONS = new Set([
	'txt',
	'md',
	'markdown',
	'csv',
	'tsv',
	'json',
	'jsonl',
	'log',
	'py',
	'go',
	'rs',
	'js',
	'jsx',
	'ts',
	'tsx',
	'html',
	'css',
	'scss',
	'xml',
	'yaml',
	'yml',
	'toml',
	'sh',
	'bash',
	'zsh',
	'sql',
	'ini',
	'conf',
	'c',
	'h',
	'cpp',
	'hpp',
	'java',
	'kt',
	'swift',
	'php',
	'rb',
	'lua',
	'dart',
	'vue',
	'svelte',
	'graphql',
	'proto',
	'diff',
	'patch',
]);

export function isTextFile(entry: Pick<FileManagerEntry, 'name' | 'type'>): boolean {
	return (
		entry.type.toLowerCase().startsWith('text/') ||
		TEXT_EXTENSIONS.has(entry.name.split('.').at(-1)?.toLowerCase() ?? '')
	);
}

export type FilesView = 'list' | 'grid';
