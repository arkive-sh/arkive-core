import { api } from '#lib/api.ts';
import {
	encryptFileMetadataForFile,
	encryptFolderMetadata,
	encryptFolderName,
} from '#lib/vault/files.ts';
import { createSearchTokens, decryptSearchFile, decryptSearchFolder } from '#lib/vault/search.ts';
import type { SearchTokenEntry } from '#lib/vault/types.ts';
import {
	clearFolderShareScopeCache,
	getFolderShareScopes,
	wrapFolderForShareScopes,
} from '#lib/shares/folder-share.ts';
import type { FileManagerEntry } from './types';

function preserveFileExtension(originalName: string, nextName: string): string {
	const originalDot = originalName.lastIndexOf('.');

	if (originalDot <= 0 || originalDot === originalName.length - 1) return nextName;

	const nextDot = nextName.lastIndexOf('.');

	if (nextDot > 0 && nextDot < nextName.length - 1) return nextName;

	return `${nextName.replace(/\.+$/, '')}${originalName.slice(originalDot)}`;
}

export async function renameFileManagerEntry(
	entry: FileManagerEntry,
	vaultId: string,
	name: string,
): Promise<void> {
	const nextName = entry.kind === 'file' ? preserveFileExtension(entry.name, name) : name;
	const tokens = await createSearchTokens(nextName, vaultId, 128);
	const searchTokens = tokens.map((token): SearchTokenEntry => ({
		token,
		field: 'name',
		weight: 10,
	}));

	if (entry.kind === 'file') {
		if (!entry.encryptedMetadata || !entry.encryptedFileKey)
			throw new Error('The encrypted file details are not loaded yet. Try again.');
		const { metadata } = await decryptSearchFile({
			id: entry.id,
			folderId: entry.parentFolderId ?? null,
			vaultId,
			encryptedMetadata: entry.encryptedMetadata,
			encryptedFileKey: entry.encryptedFileKey,
			score: 0,
			url: `/files/${entry.id}/view`,
		});
		const { encryptedMetadata } = await encryptFileMetadataForFile(
			{ ...metadata, name: nextName },
			vaultId,
			entry.id,
			entry.parentFolderId ?? null,
			entry.encryptedFileKey,
		);
		await api.post('/api/entries/rename', {
			type: 'file',
			id: entry.id,
			encryptedMetadata,
			searchTokens,
		});

		return;
	}

	if (!entry.encryptedName || !entry.encryptedMetadata)
		throw new Error('The encrypted folder details are not loaded yet. Try again.');
	const parentFolderId = entry.parentFolderId ?? null;
	const { metadata } = await decryptSearchFolder({
		id: entry.id,
		parentFolderId,
		vaultId,
		encryptedName: entry.encryptedName,
		encryptedMetadata: entry.encryptedMetadata,
		score: 0,
		url: `/folders/${entry.id}`,
	});
	const [{ encryptedMetadata: encryptedName }, { encryptedMetadata }] = await Promise.all([
		encryptFolderName({ name: nextName }, vaultId, entry.id, parentFolderId),
		encryptFolderMetadata({ ...metadata, name: nextName }, vaultId, entry.id, parentFolderId),
	]);
	clearFolderShareScopeCache();
	const scopes = await getFolderShareScopes(entry.id);
	const shareEntries = await wrapFolderForShareScopes(
		{ vaultId, folderId: entry.id, parentFolderId, encryptedName, encryptedMetadata },
		scopes,
	);
	await api.post('/api/entries/rename', {
		type: 'folder',
		id: entry.id,
		encryptedName,
		encryptedMetadata,
		searchTokens,
		shareEntries,
	});
}
