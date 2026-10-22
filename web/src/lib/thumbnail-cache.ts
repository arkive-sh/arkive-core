interface ThumbnailCache {
	get(fileId: string, thumbnailVersion: number, thumbnailSize: number): Promise<Uint8Array | null>;
}

interface ThumbnailCacheModule {
	thumbnailCache: ThumbnailCache;
}

export async function getCachedThumbnail(fileId: string, thumbnailVersion: number, thumbnailSize: number): Promise<Uint8Array | null> {
	const { thumbnailCache } = (await import('../../../core/web/static/upload/thumbnail_cache.js')) as unknown as ThumbnailCacheModule;
	return thumbnailCache.get(fileId, thumbnailVersion, thumbnailSize);
}
