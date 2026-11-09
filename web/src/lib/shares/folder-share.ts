import { api } from '#lib/api.ts';
import {
	openShareKey,
	wrapFileKeyForShare,
	wrapFolderForShare,
	wrapUploadFileKeyForShare,
} from '#lib/vault/shares.ts';
import type { ShareFolderRecord } from '#lib/vault/types.ts';

type FileRoot = {
	id: string;
	name: string;
	encryptedFileKey?: string;
	parentFolderId?: string | null;
};
type FolderRoot = {
	id: string;
	name: string;
	parentFolderId?: string | null;
	encryptedName?: string;
	encryptedMetadata?: string;
};

type ApiFolder = {
	ID: string;
	VaultID: string;
	ParentFolderID: string | null;
	EncryptedName: string;
	EncryptedMetadata: string;
};

type ApiFile = {
	ID: string;
	FolderID: string | null;
	EncryptedFileKey: string;
};

type FolderEntriesPage = {
	Folders: ApiFolder[];
	Files: ApiFile[];
	NextCursor: string | null;
	HasMore: boolean;
};

export type FolderShareScope = { id: string; token: string; encryptedShareKey: string };

const SHARE_ENTRY_BATCH_SIZE = 200;
const SHARE_SCOPE_CACHE_MS = 5000;
const scopeCache = new Map<string, { expiresAt: number; promise: Promise<FolderShareScope[]> }>();

export function clearFolderShareScopeCache(): void {
	scopeCache.clear();
}

export async function getFolderShareScopes(folderId: string): Promise<FolderShareScope[]> {
	const cached = scopeCache.get(folderId);

	if (cached && cached.expiresAt > Date.now()) return cached.promise;
	const promise = api
		.get<{ shares: FolderShareScope[] }>(
			`/api/folders/${encodeURIComponent(folderId)}/share-scopes`,
		)
		.then(({ data }) => data.shares)
		.catch((error: unknown) => {
			scopeCache.delete(folderId);

			throw error;
		});
	scopeCache.set(folderId, { expiresAt: Date.now() + SHARE_SCOPE_CACHE_MS, promise });

	return promise;
}

export async function wrapFolderForShareScopes(
	record: ShareFolderRecord,
	scopes: FolderShareScope[],
): Promise<Array<{ shareId: string; encryptedName: string; encryptedMetadata: string }>> {
	return Promise.all(
		scopes.map(async (scope) => {
			const { shareSecret } = await openShareKey(scope.encryptedShareKey, scope.token);

			try {
				return {
					shareId: scope.id,
					...(await wrapFolderForShare(record, scope.token, shareSecret)),
				};
			} finally {
				shareSecret.fill(0);
			}
		}),
	);
}

export async function wrapUploadFileKeyForScopes(
	uploadToken: string,
	fileId: string,
	scopes: FolderShareScope[],
): Promise<Array<{ shareId: string; encryptedFileKeyForShare: string }>> {
	if (scopes.length > 100) throw new Error('This folder is included in too many shared links.');

	return Promise.all(
		scopes.map(async (scope) => {
			const { shareSecret } = await openShareKey(scope.encryptedShareKey, scope.token);

			try {
				return {
					shareId: scope.id,
					encryptedFileKeyForShare: await wrapUploadFileKeyForShare(
						uploadToken,
						fileId,
						scope.token,
						shareSecret,
					),
				};
			} finally {
				shareSecret.fill(0);
			}
		}),
	);
}

export async function addEntriesToFolderShares(
	vaultId: string,
	scopes: FolderShareScope[],
	fileRoots: FileRoot[] = [],
	folderRoots: FolderRoot[] = [],
	options: { includeFolderRoots?: boolean } = {},
): Promise<void> {
	if (!scopes.length || (!fileRoots.length && !folderRoots.length)) return;
	const contexts = await Promise.all(
		scopes.map(async (scope) => ({
			scope,
			secret: (await openShareKey(scope.encryptedShareKey, scope.token)).shareSecret,
			files: [] as Array<{ id: string; encryptedFileKeyForShare: string }>,
			folders: [] as Array<{ id: string; encryptedName: string; encryptedMetadata: string }>,
		})),
	);
	const pendingFolderIds = [...folderRoots.map((folder) => folder.id)];
	const visited = new Set<string>();

	async function flush(context: (typeof contexts)[number]): Promise<void> {
		if (!context.files.length && !context.folders.length) return;
		await api.post(`/api/shares/${encodeURIComponent(context.scope.id)}/folder-entries`, {
			files: context.files,
			folders: context.folders,
		});
		context.files = [];
		context.folders = [];
	}

	async function appendFile(entry: FileRoot, folderId: string | null): Promise<void> {
		if (!entry.encryptedFileKey)
			throw new Error(`Could not prepare ${entry.name} for the folder link.`);
		for (const context of contexts) {
			context.files.push({
				id: entry.id,
				encryptedFileKeyForShare: await wrapFileKeyForShare(
					{
						vaultId,
						fileId: entry.id,
						folderId,
						encryptedFileKey: entry.encryptedFileKey,
					},
					context.scope.token,
					context.secret,
				),
			});
			if (context.files.length + context.folders.length >= SHARE_ENTRY_BATCH_SIZE)
				await flush(context);
		}
	}

	async function appendFolder(
		id: string,
		vaultId: string,
		parentFolderId: string | null,
		encryptedName: string,
		encryptedMetadata: string,
	): Promise<void> {
		for (const context of contexts) {
			const wrapped = await wrapFolderForShare(
				{ folderId: id, vaultId, parentFolderId, encryptedName, encryptedMetadata },
				context.scope.token,
				context.secret,
			);
			context.folders.push({ id, ...wrapped });
			if (context.files.length + context.folders.length >= SHARE_ENTRY_BATCH_SIZE)
				await flush(context);
		}
	}

	try {
		for (const file of fileRoots) await appendFile(file, file.parentFolderId ?? null);
		if (options.includeFolderRoots) {
			for (const folder of folderRoots) {
				if (!folder.encryptedName || !folder.encryptedMetadata)
					throw new Error(`Could not prepare ${folder.name} for the folder link.`);
				await appendFolder(
					folder.id,
					vaultId,
					folder.parentFolderId ?? null,
					folder.encryptedName,
					folder.encryptedMetadata,
				);
			}
		}

		while (pendingFolderIds.length) {
			const folderId = pendingFolderIds.pop()!;

			if (visited.has(folderId)) continue;
			visited.add(folderId);
			let cursor: string | null = null;
			do {
				const response = await api.get<FolderEntriesPage>(
					`/api/folders/${encodeURIComponent(folderId)}/entries`,
					{ params: { pageSize: 100, cursor: cursor ?? undefined } },
				);
				const data: FolderEntriesPage = response.data;

				for (const folder of data.Folders) {
					await appendFolder(
						folder.ID,
						folder.VaultID,
						folder.ParentFolderID,
						folder.EncryptedName,
						folder.EncryptedMetadata,
					);
					pendingFolderIds.push(folder.ID);
				}
				for (const file of data.Files) {
					await appendFile(
						{
							id: file.ID,
							name: 'file',
							parentFolderId: file.FolderID,
							encryptedFileKey: file.EncryptedFileKey,
						},
						file.FolderID,
					);
				}
				cursor = data.HasMore ? data.NextCursor : null;
			} while (cursor);
		}
		await Promise.all(contexts.map(flush));
	} finally {
		for (const context of contexts) context.secret.fill(0);
	}
}
