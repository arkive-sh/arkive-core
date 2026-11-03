interface ThumbnailCache {
	get(fileId: string, thumbnailVersion: number, thumbnailSize: number): Promise<Uint8Array | null>;
	put(
		fileId: string,
		thumbnailVersion: number,
		thumbnailSize: number,
		encryptedBytes: Uint8Array,
	): Promise<void>;
}

interface ThumbnailCacheModule {
	thumbnailCache: ThumbnailCache;
}

export async function getCachedThumbnail(
	fileId: string,
	thumbnailVersion: number,
	thumbnailSize: number,
): Promise<Uint8Array | null> {
	const { thumbnailCache } =
		(await import('../../../core/web/static/upload/thumbnail_cache.js')) as unknown as ThumbnailCacheModule;

	return thumbnailCache.get(fileId, thumbnailVersion, thumbnailSize);
}

export async function cacheThumbnail(
	fileId: string,
	thumbnailVersion: number,
	thumbnailSize: number,
	encryptedBytes: Uint8Array,
): Promise<void> {
	const { thumbnailCache } =
		(await import('../../../core/web/static/upload/thumbnail_cache.js')) as unknown as ThumbnailCacheModule;
	await thumbnailCache.put(fileId, thumbnailVersion, thumbnailSize, encryptedBytes);
}
