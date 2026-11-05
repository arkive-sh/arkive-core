import { isHeicImage, MAX_IMAGE_PREVIEW_BYTES } from './image';

const DECODE_TIMEOUT_MS = 15_000;

export interface DecodedHeicImage {
	blob: Blob;
	thumbnailWidth: number;
	thumbnailHeight: number;
	originalWidth: number;
	originalHeight: number;
}

// Serialize decodes so concurrent uploads cannot each reserve a 256 MiB WASM heap.
let decodeQueue = Promise.resolve();

function throwIfAborted(signal?: AbortSignal): void {
	if (signal?.aborted) throw new DOMException('HEIC decoding canceled.', 'AbortError');
}

function decodeInWorker(
	buffer: ArrayBuffer,
	maxDimension: number | null,
	signal?: AbortSignal,
): Promise<DecodedHeicImage> {
	return new Promise((resolve, reject) => {
		const worker = new Worker(new URL('./heic-decoder.worker.ts', import.meta.url), {
			type: 'module',
		});
		let finished = false;
		const timeout = setTimeout(
			() => finish(new Error('HEIC decoding timed out.')),
			DECODE_TIMEOUT_MS,
		);
		const onAbort = () => finish(new DOMException('HEIC decoding canceled.', 'AbortError'));
		const finish = (error?: Error, image?: DecodedHeicImage) => {
			if (finished) return;
			finished = true;
			clearTimeout(timeout);
			signal?.removeEventListener('abort', onAbort);
			worker.terminate();
			if (error) reject(error);
			else if (image) resolve(image);
		};

		if (signal?.aborted) {
			finish(new DOMException('HEIC decoding canceled.', 'AbortError'));

			return;
		}
		signal?.addEventListener('abort', onAbort, { once: true });
		worker.onmessage = (
			event: MessageEvent<{
				ok: boolean;
				blob?: Blob;
				thumbnailWidth?: number;
				thumbnailHeight?: number;
				originalWidth?: number;
				originalHeight?: number;
				error?: string;
			}>,
		) => {
			if (
				!event.data.ok ||
				!event.data.blob ||
				!event.data.thumbnailWidth ||
				!event.data.thumbnailHeight
			) {
				finish(new Error(event.data.error || 'HEIC/HEIF decoding failed.'));

				return;
			}
			finish(undefined, {
				blob: event.data.blob,
				thumbnailWidth: event.data.thumbnailWidth,
				thumbnailHeight: event.data.thumbnailHeight,
				originalWidth: event.data.originalWidth ?? event.data.thumbnailWidth,
				originalHeight: event.data.originalHeight ?? event.data.thumbnailHeight,
			});
		};
		worker.onerror = () => finish(new Error('The HEIC/HEIF decoder failed.'));
		// Only this selected image crosses into the decoder; vault keys and other files stay outside it.
		worker.postMessage({ buffer, maxDimension }, [buffer]);
	});
}

export function decodeHeicImage(
	source: Blob,
	name: string,
	mime: string,
	maxDimension: number | null,
	signal?: AbortSignal,
): Promise<DecodedHeicImage> {
	if (!isHeicImage(name, mime)) return Promise.reject(new Error('This is not a HEIC/HEIF image.'));
	if (source.size > MAX_IMAGE_PREVIEW_BYTES) {
		return Promise.reject(
			new Error('HEIC/HEIF files larger than 80 MB cannot be decoded by this browser.'),
		);
	}

	const decode = decodeQueue.then(async () => {
		throwIfAborted(signal);
		const buffer = await source.arrayBuffer();

		if (signal?.aborted) {
			new Uint8Array(buffer).fill(0);
			throwIfAborted(signal);
		}
		try {
			return await decodeInWorker(buffer, maxDimension, signal);
		} catch (error) {
			if (buffer.byteLength) new Uint8Array(buffer).fill(0);

			throw error;
		}
	});
	decodeQueue = decode.then(
		() => undefined,
		() => undefined,
	);

	return decode;
}
