import { CORE_API_ORIGIN } from '$app/env/public';
import { api } from '../api';
import { encryptUploadThumbnail } from '../vault/uploads';

const MAX_DIMENSION = 320;
const WEBP_QUALITY = 0.7;
const VIDEO_FRAME_SECONDS = 1;
const VIDEO_FRAME_DURATION_RATIO = 0.1;
const LONG_VIDEO_SECONDS = 30 * 60;
const LONG_VIDEO_FRAME_RATIO = 0.5;
const UPLOAD_TIMEOUT_MS = 15_000;

export interface GeneratedThumbnail {
	bytes: Uint8Array;
	mime: 'image/webp';
	width: number;
	height: number;
}

export interface UploadedThumbnail {
	mime: 'image/webp';
	width: number;
	height: number;
	encryptedSize: number;
}

interface UploadThumbnailInput {
	file: File;
	uploadToken: string;
	uploadSessionId: string;
	vaultId: string;
	fileId: string;
	signal?: AbortSignal;
}

function fitWithin(width: number, height: number): { width: number; height: number } {
	if (width <= 0 || height <= 0) return { width: 0, height: 0 };
	if (width <= MAX_DIMENSION && height <= MAX_DIMENSION) {
		return { width: Math.max(1, Math.round(width)), height: Math.max(1, Math.round(height)) };
	}
	const scale = Math.min(MAX_DIMENSION / width, MAX_DIMENSION / height);
	return {
		width: Math.max(1, Math.round(width * scale)),
		height: Math.max(1, Math.round(height * scale))
	};
}

function canvasToWebP(canvas: HTMLCanvasElement): Promise<Blob> {
	return new Promise((resolve, reject) => {
		canvas.toBlob(
			(blob) => (blob ? resolve(blob) : reject(new Error('Thumbnail encoding failed'))),
			'image/webp',
			WEBP_QUALITY
		);
	});
}

function throwIfAborted(signal?: AbortSignal): void {
	if (signal?.aborted) throw new DOMException('Thumbnail generation canceled.', 'AbortError');
}

function waitForEvent(
	target: EventTarget,
	eventName: string,
	errorName: string,
	signal?: AbortSignal
): Promise<void> {
	return new Promise((resolve, reject) => {
		const cleanup = () => {
			target.removeEventListener(eventName, onSuccess);
			target.removeEventListener(errorName, onError);
			signal?.removeEventListener('abort', onAbort);
		};
		const onSuccess = () => {
			cleanup();
			resolve();
		};
		const onError = () => {
			cleanup();
			reject(new Error('Thumbnail media load failed'));
		};
		const onAbort = () => {
			cleanup();
			reject(new DOMException('Thumbnail generation canceled.', 'AbortError'));
		};
		target.addEventListener(eventName, onSuccess, { once: true });
		target.addEventListener(errorName, onError, { once: true });
		if (signal?.aborted) onAbort();
		else signal?.addEventListener('abort', onAbort, { once: true });
	});
}

async function renderCanvasThumbnail(
	width: number,
	height: number,
	draw: (context: CanvasRenderingContext2D) => void,
	signal?: AbortSignal
): Promise<GeneratedThumbnail | null> {
	const size = fitWithin(width, height);
	if (!size.width || !size.height) return null;
	const canvas = document.createElement('canvas');
	canvas.width = size.width;
	canvas.height = size.height;
	const context = canvas.getContext('2d', { alpha: false });
	if (!context) return null;
	draw(context);
	const blob = await canvasToWebP(canvas);
	throwIfAborted(signal);
	return {
		bytes: new Uint8Array(await blob.arrayBuffer()),
		mime: 'image/webp',
		width: size.width,
		height: size.height
	};
}

async function createImageThumbnail(file: File, signal?: AbortSignal): Promise<GeneratedThumbnail | null> {
	let bitmap: ImageBitmap | HTMLImageElement | null = null;
	let objectURL = '';
	try {
		if (typeof createImageBitmap === 'function') {
			bitmap = await createImageBitmap(file);
		} else {
			objectURL = URL.createObjectURL(file);
			const image = new Image();
			const imageLoaded = waitForEvent(image, 'load', 'error', signal);
			image.src = objectURL;
			await imageLoaded;
			bitmap = image;
		}
		throwIfAborted(signal);
		return await renderCanvasThumbnail(
			bitmap.width,
			bitmap.height,
			(context) => context.drawImage(bitmap!, 0, 0, context.canvas.width, context.canvas.height),
			signal
		);
	} finally {
		if (bitmap && 'close' in bitmap) bitmap.close();
		if (objectURL) URL.revokeObjectURL(objectURL);
	}
}

async function createVideoThumbnail(file: File, signal?: AbortSignal): Promise<GeneratedThumbnail | null> {
	const objectURL = URL.createObjectURL(file);
	const video = document.createElement('video');
	video.preload = 'metadata';
	video.muted = true;
	video.playsInline = true;
	video.crossOrigin = 'anonymous';
	try {
		const metadataReady = waitForEvent(video, 'loadedmetadata', 'error', signal);
		video.src = objectURL;
		await metadataReady;
		if (video.readyState < HTMLMediaElement.HAVE_CURRENT_DATA) {
			await waitForEvent(video, 'loadeddata', 'error', signal);
		}
		const duration = Math.max(0, Number(video.duration || 0));
		const targetTime = duration >= LONG_VIDEO_SECONDS
			? duration * LONG_VIDEO_FRAME_RATIO
			: Math.min(VIDEO_FRAME_SECONDS, duration * VIDEO_FRAME_DURATION_RATIO);
		const seekTime = Math.max(0, Math.min(targetTime, duration));
		if (Math.abs(video.currentTime - seekTime) >= 0.05) {
			const seeked = waitForEvent(video, 'seeked', 'error', signal);
			video.currentTime = seekTime;
			await seeked;
		}
		throwIfAborted(signal);
		return await renderCanvasThumbnail(
			video.videoWidth,
			video.videoHeight,
			(context) => {
				const size = fitWithin(video.videoWidth, video.videoHeight);
				context.drawImage(video, 0, 0, size.width, size.height);
			},
			signal
		);
	} finally {
		video.pause();
		video.removeAttribute('src');
		video.load();
		URL.revokeObjectURL(objectURL);
	}
}

export async function createThumbnail(file: File, signal?: AbortSignal): Promise<GeneratedThumbnail | null> {
	const mime = file.type.toLowerCase();
	try {
		if (mime.startsWith('image/')) return await createImageThumbnail(file, signal);
		if (mime.startsWith('video/')) return await createVideoThumbnail(file, signal);
		return null;
	} catch (error) {
		if (typeof window !== 'undefined' && window.location.hostname === 'localhost') {
			console.warn('Thumbnail generation failed:', error instanceof Error ? error.message : 'Unknown error');
		}
		return null;
	}
}

export async function createAndUploadThumbnail(input: UploadThumbnailInput): Promise<UploadedThumbnail | null> {
	const controller = new AbortController();
	const abort = () => controller.abort();
	const timeout = setTimeout(abort, UPLOAD_TIMEOUT_MS);
	if (input.signal?.aborted) controller.abort();
	else input.signal?.addEventListener('abort', abort, { once: true });
	let thumbnail: GeneratedThumbnail | null = null;
	let encryptedThumbnail: Uint8Array | null = null;
	try {
		thumbnail = await createThumbnail(input.file, controller.signal);
		if (!thumbnail || controller.signal.aborted) return null;
		const encrypted = await encryptUploadThumbnail(
			input.uploadToken,
			thumbnail.bytes,
			`arkive:file-thumbnail:v1:${input.vaultId}:${input.fileId}`
		);
		encryptedThumbnail = encrypted.encryptedThumbnail;
		if (controller.signal.aborted) return null;
		const { data } = await api.post<{ url: string }>(
			`/api/uploads/${encodeURIComponent(input.uploadSessionId)}/thumbnail/presign`,
			{ encryptedSize: encrypted.encryptedSize, mime: thumbnail.mime, width: thumbnail.width, height: thumbnail.height },
			{ signal: controller.signal }
		);
		const response = await fetch(new URL(data.url, CORE_API_ORIGIN), {
			method: 'PUT',
			headers: { 'Content-Type': 'application/octet-stream' },
			body: encryptedThumbnail,
			signal: controller.signal
		});
		if (!response.ok) throw new Error(`Thumbnail upload failed (${response.status}).`);
		return {
			mime: thumbnail.mime,
			width: thumbnail.width,
			height: thumbnail.height,
			encryptedSize: encrypted.encryptedSize
		};
	} catch (error) {
		if (typeof window !== 'undefined' && window.location.hostname === 'localhost') {
			console.warn('Thumbnail upload failed:', error instanceof Error ? error.message : 'Unknown error');
		}
		return null;
	} finally {
		clearTimeout(timeout);
		input.signal?.removeEventListener('abort', abort);
		thumbnail?.bytes.fill(0);
		encryptedThumbnail?.fill(0);
	}
}
