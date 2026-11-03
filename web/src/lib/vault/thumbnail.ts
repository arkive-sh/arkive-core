import { api } from '#lib/api.ts';
import { cacheThumbnail, getCachedThumbnail } from '#lib/thumbnail-cache.ts';
import { decryptSearchThumbnail } from './search';
import type { EncryptedSearchFile } from './types';

export interface EncryptedThumbnailInput {
	id: string;
	version: number;
	size: number;
	mime: string;
	vaultId: string;
	encryptedMetadata: string;
	encryptedFileKey: string;
}

export async function readDecryptedThumbnail(
	file: EncryptedThumbnailInput,
	signal: AbortSignal,
	isCurrent: () => boolean = () => true,
): Promise<Blob | null> {
	let encryptedBytes = await getCachedThumbnail(file.id, file.version, file.size);

	if (signal.aborted || !isCurrent()) return null;
	if (!encryptedBytes) {
		const { data } = await api.get<{ url: string }>(
			`/api/files/${encodeURIComponent(file.id)}/thumbnail/presign`,
			{ signal },
		);
		const response = await fetch(data.url, { credentials: 'omit', signal });

		if (!response.ok) throw new Error(`Thumbnail request failed (${response.status}).`);
		if (signal.aborted || !isCurrent()) return null;
		encryptedBytes = new Uint8Array(await response.arrayBuffer());
		void cacheThumbnail(file.id, file.version, file.size, encryptedBytes).catch(() => {});
	}

	const searchFile: EncryptedSearchFile = {
		id: file.id,
		folderId: null,
		vaultId: file.vaultId,
		encryptedMetadata: file.encryptedMetadata,
		encryptedFileKey: file.encryptedFileKey,
		score: 0,
		url: `/files/${file.id}/view`,
	};
	const { thumbnailBytes } = await decryptSearchThumbnail(searchFile, encryptedBytes);

	try {
		if (signal.aborted || !isCurrent() || !thumbnailBytes.length) return null;
		const bytes = thumbnailBytes.slice();

		try {
			return new Blob([bytes.buffer as ArrayBuffer], { type: file.mime });
		} finally {
			bytes.fill(0);
		}
	} finally {
		thumbnailBytes.fill(0);
	}
}
