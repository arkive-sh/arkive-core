const IMAGE_MIME_BY_EXTENSION: Record<string, string> = {
	avif: 'image/avif',
	bmp: 'image/bmp',
	gif: 'image/gif',
	heic: 'image/heic',
	heif: 'image/heif',
	jpeg: 'image/jpeg',
	jpg: 'image/jpeg',
	png: 'image/png',
	webp: 'image/webp',
};

export const MAX_IMAGE_PREVIEW_BYTES = 80 * 1024 * 1024;

export function imageMimeType(name: string, mime = ''): string {
	if (mime.toLowerCase().startsWith('image/')) return mime;
	const extension = name.split('.').pop()?.toLowerCase() ?? '';

	return IMAGE_MIME_BY_EXTENSION[extension] ?? '';
}

export function isImageFile(name: string, mime = ''): boolean {
	return imageMimeType(name, mime) !== '';
}

export function isHeicImage(name: string, mime = ''): boolean {
	const normalizedMime = mime.toLowerCase();
	const extension = name.split('.').pop()?.toLowerCase() ?? '';

	return (
		extension === 'heic' ||
		extension === 'heif' ||
		normalizedMime === 'image/heic' ||
		normalizedMime === 'image/heif' ||
		normalizedMime === 'image/heic-sequence' ||
		normalizedMime === 'image/heif-sequence'
	);
}
