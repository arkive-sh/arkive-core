type DecodeRequest = {
	buffer: ArrayBuffer;
	maxDimension: number | null;
};

type HeifImage = {
	is_primary(): boolean;
	get_width(): number;
	get_height(): number;
	display(imageData: ImageData, callback: (image: ImageData | null) => void): void;
	free(): void;
};

type HeifDecoder = {
	decoder: number | null;
	decode(input: Uint8Array): HeifImage[];
};

type HeifModule = {
	HeifDecoder: new () => HeifDecoder;
	heif_context_free(context: number): void;
};

const MAX_HEIC_DIMENSION = 8192;
const moduleURL = new URL('/wasm/heic/libheif.js', self.location.origin).href;
const workerScope = self as unknown as {
	addEventListener(type: 'message', listener: (event: MessageEvent<DecodeRequest>) => void): void;
	postMessage(message: unknown): void;
};

async function loadLibheif(): Promise<HeifModule> {
	const { default: initialize } = await import(/* @vite-ignore */ moduleURL);

	return initialize({
		locateFile: (path: string) => new URL(path, moduleURL).href,
	}) as Promise<HeifModule>;
}

workerScope.addEventListener('message', async (event: MessageEvent<DecodeRequest>) => {
	const input = new Uint8Array(event.data.buffer);
	let module: HeifModule | undefined;
	let decoder: HeifDecoder | undefined;
	let images: HeifImage[] = [];
	let pixels: ImageData | undefined;
	let resized: ImageBitmap | undefined;
	let canvas: OffscreenCanvas | undefined;

	try {
		module = await loadLibheif();
		decoder = new module.HeifDecoder();
		images = decoder.decode(input);
		if (!decoder.decoder || images.length === 0)
			throw new Error('This HEIC/HEIF image could not be decoded.');

		const image = images.find((candidate) => candidate.is_primary()) ?? images[0];
		const width = image.get_width();
		const height = image.get_height();

		if (
			!Number.isSafeInteger(width) ||
			!Number.isSafeInteger(height) ||
			width <= 0 ||
			height <= 0
		) {
			throw new Error('This HEIC/HEIF image has invalid dimensions.');
		}
		if (width > MAX_HEIC_DIMENSION || height > MAX_HEIC_DIMENSION) {
			throw new Error(
				'HEIC/HEIF images larger than 8K on either side cannot be decoded by this browser.',
			);
		}

		const scale = event.data.maxDimension
			? Math.min(1, event.data.maxDimension / Math.max(width, height))
			: 1;
		const outputWidth = Math.max(1, Math.round(width * scale));
		const outputHeight = Math.max(1, Math.round(height * scale));
		pixels = new ImageData(width, height);
		await new Promise<void>((resolve, reject) => {
			image.display(pixels!, (decoded) =>
				decoded ? resolve() : reject(new Error('This HEIC/HEIF image could not be decoded.')),
			);
		});
		resized = await createImageBitmap(pixels, {
			resizeWidth: outputWidth,
			resizeHeight: outputHeight,
			resizeQuality: 'high',
		});
		pixels.data.fill(0);

		canvas = new OffscreenCanvas(outputWidth, outputHeight);
		const context = canvas.getContext('2d', { alpha: false });

		if (!context) throw new Error('HEIC/HEIF preview rendering is unavailable.');
		context.drawImage(resized, 0, 0);
		const blob = await canvas.convertToBlob({
			type: 'image/webp',
			quality: event.data.maxDimension ? 0.7 : 0.9,
		});
		workerScope.postMessage({
			ok: true,
			blob,
			thumbnailWidth: outputWidth,
			thumbnailHeight: outputHeight,
			originalWidth: width,
			originalHeight: height,
		});
	} catch (error) {
		workerScope.postMessage({
			ok: false,
			error: error instanceof Error ? error.message : 'HEIC/HEIF decoding failed.',
		});
	} finally {
		resized?.close();
		pixels?.data.fill(0);
		if (canvas) {
			canvas.width = 0;
			canvas.height = 0;
		}
		for (const image of images) image.free();
		if (module && decoder?.decoder) module.heif_context_free(decoder.decoder);
		input.fill(0);
	}
});
