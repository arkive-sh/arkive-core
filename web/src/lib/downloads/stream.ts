import { openEncryptedStream } from '#lib/streaming/source.ts';
import type { EncryptedStreamOptions } from '#lib/streaming/types.ts';

export async function streamDownload(
	options: EncryptedStreamOptions & {
		signal: AbortSignal;
		target: FileSystemFileHandle;
		onProgress: (transferredBytes: number) => void;
	},
): Promise<void> {
	const { signal, target, onProgress, ...streamOptions } = options;
	const stream = await openEncryptedStream({
		...streamOptions,
		purpose: 'download',
	});

	try {
		signal.throwIfAborted();
		const response = await fetch(stream.url, { signal, cache: 'no-store' });

		if (!response.ok)
			throw new Error((await response.text()) || `Download failed (${response.status}).`);
		if (!response.body) throw new Error('The download response has no file data.');

		let transferredBytes = 0;
		const file = await target.createWritable();
		await response.body.pipeTo(
			new WritableStream<Uint8Array<ArrayBuffer>>({
				async write(chunk) {
					await file.write(chunk);
					transferredBytes += chunk.byteLength;
					onProgress(transferredBytes);
				},
				close: () => file.close(),
				abort: (reason) => file.abort(reason),
			}),
			{ signal },
		);
	} finally {
		stream.close();
	}
}
