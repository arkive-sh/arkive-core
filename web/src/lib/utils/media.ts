function canStreamNativeMedia(
	mimeType: string,
	type: 'audio' | 'video',
	createMedia: () => HTMLMediaElement,
): boolean {
	if (
		typeof window === 'undefined' ||
		typeof document === 'undefined' ||
		!window.isSecureContext ||
		!('serviceWorker' in navigator)
	)
		return false;
	if (!mimeType.toLowerCase().startsWith(`${type}/`)) return false;

	return createMedia().canPlayType(mimeType) !== '';
}

export function isAudioMimeType(mimeType: string): boolean {
	return mimeType.toLowerCase().startsWith('audio/');
}

export function isMediaMimeType(mimeType: string): boolean {
	const type = mimeType.toLowerCase();

	return type.startsWith('audio/') || type.startsWith('video/');
}

export function canStreamAudio(mimeType: string): boolean {
	return canStreamNativeMedia(mimeType, 'audio', () => document.createElement('audio'));
}

export function canStreamVideo(mimeType: string): boolean {
	return canStreamNativeMedia(mimeType, 'video', () => document.createElement('video'));
}
