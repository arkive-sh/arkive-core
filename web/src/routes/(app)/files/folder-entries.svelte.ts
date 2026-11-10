import { createInfiniteQuery } from '@tanstack/svelte-query';
import { SvelteMap } from 'svelte/reactivity';
import { api } from '#lib/api.ts';
import { imageMimeType } from '#lib/utils/image.ts';
import { decryptSearchFile, decryptSearchFolder } from '#lib/vault/search.ts';
import { fileManagerEntryKey, type FileManagerEntry } from './types';

export const FOLDER_ENTRIES_PAGE_SIZE = 50;

export type ApiFolder = {
	ID: string;
	VaultID: string;
	ParentFolderID: string | null;
	EncryptedName: string;
	EncryptedMetadata: string;
	SizeBytes?: number;
	UpdatedAt: string;
};

export type ApiFile = {
	ID: string;
	FolderID: string | null;
	EncryptedMetadata: string;
	EncryptedFileKey: string;
	UpdatedAt: string;
	IsShared: boolean;
};

export type FolderEntriesResponse = {
	FolderID: string | null;
	Path: ApiFolder[];
	Folders: ApiFolder[];
	Files: ApiFile[];
	NextCursor: string | null;
	HasMore: boolean;
};

export function createFolderEntries(
	getVaultId: () => string,
	getFolderId: () => string | null,
	getFocusFileId: () => string | null,
	getVaultUnlocked: () => boolean,
) {
	let rows = $state<FileManagerEntry[]>([]);
	let breadcrumbs = $state<Array<{ id: string | null; name: string }>>([
		{ id: null, name: 'Root' },
	]);
	let entriesReady = $state(false);
	let decryptGeneration = 0;
	let hydratedPageCount = 0;
	let hydratedFolderId: string | null | undefined;
	let hydratedFirstPage: FolderEntriesResponse | undefined;
	let hydrationQueue = Promise.resolve();

	const query = createInfiniteQuery(() => {
		const folderId = getFolderId();
		const focusFileId = getFocusFileId();

		return {
			queryKey: ['folder-entries', folderId, focusFileId],
			enabled: getVaultUnlocked(),
			initialPageParam: null as string | null,
			queryFn: async ({ pageParam, signal }) => {
				const endpoint = folderId
					? `/api/folders/${encodeURIComponent(folderId)}/entries`
					: '/api/folders/root/entries';
				const { data: result } = await api.get<FolderEntriesResponse>(endpoint, {
					params: {
						cursor: pageParam ?? undefined,
						pageSize: FOLDER_ENTRIES_PAGE_SIZE,
						seekEntryId: pageParam === null ? (focusFileId ?? undefined) : undefined,
					},
					signal,
				});

				return result;
			},
			getNextPageParam: (lastPage) =>
				lastPage.HasMore ? lastPage.NextCursor || undefined : undefined,
		};
	});

	const loadedEntries = $derived([
		...new SvelteMap<string, FileManagerEntry>(
			[
				...(query.data?.pages ?? []).flatMap((response) => [
					...response.Folders.map((folder): FileManagerEntry => ({
						id: folder.ID,
						kind: 'folder',
						name: 'Encrypted folder',
						type: 'Folder',
						size: folder.SizeBytes ?? null,
						updatedAt: folder.UpdatedAt,
					})),
					...response.Files.map((file): FileManagerEntry => ({
						id: file.ID,
						kind: 'file',
						name: 'Encrypted file',
						type: 'application/octet-stream',
						size: null,
						updatedAt: file.UpdatedAt,
					})),
				]),
				...rows,
			].map((entry) => [fileManagerEntryKey(entry), entry] as const),
		).values(),
	]);

	async function decryptEntries(
		response: FolderEntriesResponse,
		generation: number,
		setPath: boolean,
	): Promise<
		| { rows: FileManagerEntry[]; breadcrumbs?: Array<{ id: string | null; name: string }> }
		| undefined
	> {
		const [folderRows, fileRows] = await Promise.all([
			Promise.all(
				response.Folders.map(async (folder): Promise<FileManagerEntry> => {
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
							encryptedName: folder.EncryptedName,
							encryptedMetadata: folder.EncryptedMetadata,
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
							encryptedName: folder.EncryptedName,
							encryptedMetadata: folder.EncryptedMetadata,
						};
					}
				}),
			),
			Promise.all(
				response.Files.map(async (file): Promise<FileManagerEntry> => {
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
						const preview = decrypted.metadata.preview;
						const thumbnailVersion = Number(preview?.thumbnail_version || 0);
						const thumbnailSize = Number(preview?.thumbnail_size || 0);
						const dimensions = decrypted.metadata.dimensions;
						const mediaDimensions =
							dimensions &&
							Number.isSafeInteger(dimensions.width) &&
							dimensions.width > 0 &&
							Number.isSafeInteger(dimensions.height) &&
							dimensions.height > 0
								? { width: dimensions.width, height: dimensions.height }
								: undefined;
						const storedMime =
							typeof decrypted.metadata.mime === 'string' ? decrypted.metadata.mime : '';
						const mime =
							imageMimeType(decrypted.metadata.name || '', storedMime) ||
							storedMime ||
							'application/octet-stream';

						return {
							id: file.ID,
							kind: 'file',
							name: decrypted.metadata.name || 'Encrypted file',
							type: mime,
							thumbnail:
								preview?.has_thumbnail && thumbnailVersion > 0 && thumbnailSize > 0
									? {
											version: thumbnailVersion,
											size: thumbnailSize,
											mime: preview.thumbnail_mime || 'image/webp',
											thumbnailWidth: Number(preview.thumbnail_width || 0),
											thumbnailHeight: Number(preview.thumbnail_height || 0),
											vaultId: getVaultId(),
											folderId: file.FolderID,
											encryptedMetadata: file.EncryptedMetadata,
											encryptedFileKey: file.EncryptedFileKey,
										}
									: undefined,
							size: typeof decrypted.metadata.size === 'number' ? decrypted.metadata.size : null,
							updatedAt: file.UpdatedAt,
							isShared: file.IsShared,
							parentFolderId: file.FolderID,
							encryptedFileKey: file.EncryptedFileKey,
							dimensions: mediaDimensions,
						};
					} catch {
						return {
							id: file.ID,
							kind: 'file',
							name: 'Encrypted file',
							type: 'application/octet-stream',
							size: null,
							updatedAt: file.UpdatedAt,
							isShared: file.IsShared,
							parentFolderId: file.FolderID,
							encryptedFileKey: file.EncryptedFileKey,
						};
					}
				}),
			),
		]);

		const path = setPath
			? await Promise.all(
					response.Path.map(async (folder) => {
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

							return { id: folder.ID, name: decrypted.name.name || 'Encrypted folder' };
						} catch {
							return { id: folder.ID, name: 'Encrypted folder' };
						}
					}),
				)
			: undefined;

		if (generation !== decryptGeneration || !getVaultUnlocked()) return;

		return {
			rows: [...folderRows, ...fileRows],
			breadcrumbs: path ? [{ id: null, name: 'Root' }, ...path] : undefined,
		};
	}

	$effect(() => {
		const activeFolderId = getFolderId();
		const pages = query.data?.pages ?? [];

		if (activeFolderId !== hydratedFolderId) {
			hydratedFolderId = activeFolderId;
			decryptGeneration += 1;
			hydratedPageCount = 0;
			hydratedFirstPage = undefined;
			hydrationQueue = Promise.resolve();
			rows = [];
			breadcrumbs = [{ id: null, name: 'Root' }];
			entriesReady = false;
		}
		if (
			pages[0] &&
			(pages[0].FolderID?.toLowerCase() ?? null) !== (activeFolderId?.toLowerCase() ?? null)
		)
			return;
		if (!getVaultUnlocked()) {
			decryptGeneration += 1;
			hydratedPageCount = 0;
			hydrationQueue = Promise.resolve();
			rows = [];
			breadcrumbs = [{ id: null, name: 'Root' }];
			entriesReady = false;

			return;
		}
		if (pages.length < hydratedPageCount || pages[0] !== hydratedFirstPage) {
			const keepCurrentRows = Boolean(pages[0]) && entriesReady;
			decryptGeneration += 1;
			hydratedPageCount = 0;
			hydratedFirstPage = pages[0];
			hydrationQueue = Promise.resolve();
			if (keepCurrentRows) {
				const generation = decryptGeneration;
				void (async () => {
					const refreshedRows: FileManagerEntry[] = [];
					let refreshedBreadcrumbs: Array<{ id: string | null; name: string }> | undefined;

					for (const [index, response] of pages.entries()) {
						const decrypted = await decryptEntries(response, generation, index === 0);

						if (!decrypted) return;
						refreshedRows.push(...decrypted.rows);
						if (decrypted.breadcrumbs) refreshedBreadcrumbs = decrypted.breadcrumbs;
					}
					if (generation !== decryptGeneration || !getVaultUnlocked()) return;
					rows = refreshedRows;
					if (refreshedBreadcrumbs) breadcrumbs = refreshedBreadcrumbs;
					hydratedPageCount = pages.length;
				})();

				return;
			}
			rows = [];
			breadcrumbs = [{ id: null, name: 'Root' }];
			entriesReady = false;
		}

		const generation = decryptGeneration;
		const start = hydratedPageCount;
		const newPages = pages.slice(start);
		hydratedPageCount = pages.length;
		for (const [index, response] of newPages.entries()) {
			hydrationQueue = hydrationQueue.then(async () => {
				const decrypted = await decryptEntries(response, generation, start === 0 && index === 0);

				if (!decrypted) return;
				rows = [...rows, ...decrypted.rows];
				if (decrypted.breadcrumbs) breadcrumbs = decrypted.breadcrumbs;
				entriesReady = true;
			});
		}
	});

	return {
		query,
		get rows() {
			return rows;
		},
		get breadcrumbs() {
			return breadcrumbs;
		},
		get entriesReady() {
			return entriesReady;
		},
		get loadedEntries() {
			return loadedEntries;
		},
	};
}
