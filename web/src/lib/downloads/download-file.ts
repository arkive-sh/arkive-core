import { DecryptedFileReader } from '#lib/vault/read-file.ts';
import { streamDownload } from './stream.ts';

export async function downloadEncryptedFile(
	fileId: string,
	getTarget: () => Promise<FileSystemFileHandle>,
	signal: AbortSignal,
	onProgress: (transferredBytes: number) => void,
	concurrency: number,
): Promise<void> {
	const reader = new DecryptedFileReader(fileId, signal);

	try {
		const { record, metadata } = await reader.load();
		await streamDownload({
			reader,
			record,
			metadata,
			target: await getTarget(),
			signal,
			onProgress,
			concurrency,
		});
	} finally {
		await reader.dispose();
	}
}
