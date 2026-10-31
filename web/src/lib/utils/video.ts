export function canStreamVideo(mimeType: string): boolean {
	if (typeof window === 'undefined' || typeof document === 'undefined' || !window.isSecureContext || !('serviceWorker' in navigator)) return false;
	if (!mimeType.toLowerCase().startsWith('video/')) return false;
	return document.createElement('video').canPlayType(mimeType) !== '';
}
