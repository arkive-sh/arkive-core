import type { FileManagerEntry } from '../../routes/(app)/files/types';
import { api } from '#lib/api.ts';
import { decodeJSON } from '#lib/vault/bytes.ts';
import { openPublicShareFileMetadata } from '#lib/vault/shares.ts';
import { decryptSearchFile, decryptSearchFolder } from '#lib/vault/search.ts';
import type { VaultMetadata } from '#lib/vault/types.ts';
import type { FolderEntriesResponse } from '../../routes/(app)/files/folder-entries.svelte';

export type FolderTreePage = {
	payload: unknown;
	nextCursor: string | null;
	hasMore: boolean;
};

export type FolderTreeSource = {
	queryKey: (parentFolderId: string | null) => readonly unknown[];
	loadPage: (
		parentFolderId: string | null,
		cursor: string | null,
		signal: AbortSignal,
	) => Promise<FolderTreePage>;
	decryptPage: (payload: unknown, parentFolderId: string | null) => Promise<FileManagerEntry[]>;
};

export function createFilesFolderTreeSource(getVaultId: () => string): FolderTreeSource {
	return {
		queryKey: (parentFolderId) => ['folder-entries', parentFolderId, null, 'tree'],
		loadPage: async (parentFolderId, cursor, signal) => {
			const endpoint = parentFolderId
				? `/api/folders/${encodeURIComponent(parentFolderId)}/entries`
				: '/api/folders/root/entries';
			const { data } = await api.get<FolderEntriesResponse>(endpoint, {
				params: { cursor: cursor ?? undefined, pageSize: 50 },
				signal,
			});

			return { payload: data, nextCursor: data.NextCursor, hasMore: data.HasMore };
		},
		decryptPage: async (payload) => {
			const page = payload as FolderEntriesResponse;
			const folders = await Promise.all(
				page.Folders.map(async (folder): Promise<FileManagerEntry> => {
					try {
						const decrypted = await decryptSearchFolder({
							id: folder.ID,
							parentFolderId: folder.ParentFolderID,
							vaultId: getVaultId(),
							encryptedName: folder.EncryptedName,
							encryptedMetadata: folder.EncryptedMetadata,
							score: 0,
							url: `/folders/${folder.ID}`,
						});

						return {
							id: folder.ID,
							kind: 'folder',
							name: decrypted.name.name || 'Encrypted folder',
							type: 'Folder',
							size: folder.SizeBytes ?? null,
							updatedAt: folder.UpdatedAt,
							parentFolderId: folder.ParentFolderID,
						};
					} catch {
						return {
							id: folder.ID,
							kind: 'folder',
							name: 'Encrypted folder',
							type: 'Folder',
							size: folder.SizeBytes ?? null,
							updatedAt: folder.UpdatedAt,
							parentFolderId: folder.ParentFolderID,
						};
					}
				}),
			);
			const files = await Promise.all(
				page.Files.map(async (file): Promise<FileManagerEntry> => {
					try {
						const decrypted = await decryptSearchFile({
							id: file.ID,
							folderId: file.FolderID,
							vaultId: getVaultId(),
							encryptedMetadata: file.EncryptedMetadata,
							encryptedFileKey: file.EncryptedFileKey,
							score: 0,
							url: `/files/${file.ID}/view`,
						});

						return {
							id: file.ID,
							kind: 'file',
							name: decrypted.metadata.name || 'Encrypted file',
							type:
								typeof decrypted.metadata.mime === 'string'
									? decrypted.metadata.mime
									: 'application/octet-stream',
							size: typeof decrypted.metadata.size === 'number' ? decrypted.metadata.size : null,
							updatedAt: file.UpdatedAt,
							parentFolderId: file.FolderID,
						};
					} catch {
						return {
							id: file.ID,
							kind: 'file',
							name: 'Encrypted file',
							type: 'application/octet-stream',
							size: null,
							updatedAt: file.UpdatedAt,
							parentFolderId: file.FolderID,
						};
					}
				}),
			);

			return [...folders, ...files];
		},
	};
}

type ApiTrashEntry = {
	type: 'file' | 'folder';
	id: string;
	parentFolderId?: string | null;
	encryptedName?: string;
	encryptedMetadata?: string;
	encryptedFileKey?: string;
	deletedAt: string;
	isContainer?: boolean;
};

type TrashPage = {
	entries: ApiTrashEntry[];
	nextCursor: string | null;
	hasMore: boolean;
};

export function createTrashFolderTreeSource(getVaultId: () => string): FolderTreeSource {
	return {
		queryKey: (parentFolderId) => ['trash', parentFolderId, null, 'tree'],
		loadPage: async (parentFolderId, cursor, signal) => {
			const { data } = await api.get<TrashPage>('/api/trash/entries', {
				params: {
					limit: 50,
					cursor: cursor ?? undefined,
					parentFolderId: parentFolderId ?? undefined,
				},
				signal,
			});

			return { payload: data, nextCursor: data.nextCursor, hasMore: data.hasMore };
		},
		decryptPage: async (payload) => {
			const page = payload as TrashPage;

			return Promise.all(
				page.entries.map(async (entry): Promise<FileManagerEntry> => {
					try {
						if (entry.type === 'folder') {
							if (!entry.encryptedName || !entry.encryptedMetadata) throw new Error();
							const decrypted = await decryptSearchFolder({
								id: entry.id,
								parentFolderId: entry.parentFolderId ?? null,
								vaultId: getVaultId(),
								encryptedName: entry.encryptedName,
								encryptedMetadata: entry.encryptedMetadata,
								score: 0,
								url: `/folders/${entry.id}`,
							});

							return {
								id: entry.id,
								kind: 'folder',
								name: decrypted.name.name || 'Encrypted folder',
								type: 'Folder',
								size: null,
								updatedAt: entry.deletedAt,
								parentFolderId: entry.parentFolderId,
								isTrashContainer: entry.isContainer,
							};
						}
						if (!entry.encryptedMetadata || !entry.encryptedFileKey) throw new Error();
						const decrypted = await decryptSearchFile({
							id: entry.id,
							folderId: entry.parentFolderId ?? null,
							vaultId: getVaultId(),
							encryptedMetadata: entry.encryptedMetadata,
							encryptedFileKey: entry.encryptedFileKey,
							score: 0,
							url: `/files/${entry.id}/view`,
						});

						return {
							id: entry.id,
							kind: 'file',
							name: decrypted.metadata.name || 'Encrypted file',
							type:
								typeof decrypted.metadata.mime === 'string'
									? decrypted.metadata.mime
									: 'application/octet-stream',
							size: typeof decrypted.metadata.size === 'number' ? decrypted.metadata.size : null,
							updatedAt: entry.deletedAt,
							parentFolderId: entry.parentFolderId,
						};
					} catch {
						return {
							id: entry.id,
							kind: entry.type,
							name: entry.type === 'folder' ? 'Encrypted folder' : 'Encrypted file',
							type: entry.type === 'folder' ? 'Folder' : 'application/octet-stream',
							size: null,
							updatedAt: entry.deletedAt,
							parentFolderId: entry.parentFolderId,
							isTrashContainer: entry.type === 'folder' && entry.isContainer,
						};
					}
				}),
			);
		},
	};
}

export type PublicFolder = {
	id: string;
	vaultId: string;
	parentFolderId?: string | null;
	encryptedName: string;
	encryptedMetadata: string;
	updatedAt: string;
};

export type PublicFile = {
	id: string;
	vaultId: string;
	folderId?: string | null;
	encryptedMetadata: string;
	encryptedFileKeyForShare: string;
	updatedAt: string;
};

export type PublicFolderEntries = {
	folders: PublicFolder[];
	files: PublicFile[];
	nextCursor: string;
};

export function createPublicShareFolderTreeSource(
	getToken: () => string,
	getShareKey: () => Uint8Array,
	decryptFolderName: (
		id: string,
		encryptedName: string,
		encryptedMetadata: string,
	) => Promise<string>,
): FolderTreeSource {
	async function decryptFolder(
		folder: PublicFolder,
		parentFolderId: string | null,
	): Promise<FileManagerEntry> {
		try {
			return {
				id: folder.id,
				kind: 'folder',
				name: await decryptFolderName(folder.id, folder.encryptedName, folder.encryptedMetadata),
				type: 'Folder',
				size: null,
				updatedAt: folder.updatedAt,
				parentFolderId: folder.parentFolderId ?? parentFolderId,
			};
		} catch {
			return {
				id: folder.id,
				kind: 'folder',
				name: 'Encrypted folder',
				type: 'Folder',
				size: null,
				updatedAt: folder.updatedAt,
				parentFolderId: folder.parentFolderId ?? parentFolderId,
			};
		}
	}

	return {
		queryKey: (parentFolderId) => ['public-folder-share-tree', getToken(), parentFolderId],
		loadPage: async (parentFolderId, cursor, signal) => {
			if (!parentFolderId) throw new Error('The shared folder is unavailable.');
			const { data } = await api.get<PublicFolderEntries>(
				`/api/public/shares/${encodeURIComponent(getToken())}/folders/${encodeURIComponent(parentFolderId)}/entries`,
				{ params: { pageSize: 50, cursor: cursor ?? undefined }, signal },
			);

			return {
				payload: data,
				nextCursor: data.nextCursor || null,
				hasMore: Boolean(data.nextCursor),
			};
		},
		decryptPage: async (payload, parentFolderId) => {
			const data = payload as PublicFolderEntries;
			const folders = await Promise.all(
				data.folders.map((folder) => decryptFolder(folder, parentFolderId)),
			);
			const files = await Promise.all(
				data.files.map(async (file): Promise<FileManagerEntry> => {
					try {
						const decrypted = await openPublicShareFileMetadata(
							{
								token: getToken(),
								fileId: file.id,
								vaultId: file.vaultId,
								encryptedFileKeyForShare: file.encryptedFileKeyForShare,
								encryptedMetadata: file.encryptedMetadata,
							},
							getShareKey().slice(),
						);
						const metadata = decodeJSON<VaultMetadata>(decrypted.metadataBytes);

						return {
							id: file.id,
							kind: 'file',
							name: typeof metadata.name === 'string' ? metadata.name : 'Shared file',
							type: typeof metadata.mime === 'string' ? metadata.mime : 'application/octet-stream',
							size: typeof metadata.size === 'number' ? metadata.size : null,
							updatedAt: file.updatedAt,
							parentFolderId: file.folderId ?? parentFolderId,
						};
					} catch {
						return {
							id: file.id,
							kind: 'file',
							name: 'Encrypted file',
							type: 'application/octet-stream',
							size: null,
							updatedAt: file.updatedAt,
							parentFolderId: file.folderId ?? parentFolderId,
						};
					}
				}),
			);

			return [...folders, ...files];
		},
	};
}
