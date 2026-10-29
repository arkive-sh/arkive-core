import { api } from '#lib/api.ts';
import { closeFileContext, decryptFileChunk, openFileContext } from './files.ts';

interface EncryptedFileRecord {
	fileId: string;
	vaultId: string;
	chunkSize: number;
	totalChunks: number;
	plaintextSize: number;
	encryptedMetadata: string;
	encryptedFileKey: string;
	encryptedManifest: string;
	sourceUrl: string;
}

interface FileManifest {
	chunks: Array<{ cipher_size: number; hash: string }>;
}

export async function readDecryptedFile(fileId: string, mimeType: string, signal: AbortSignal): Promise<Blob> {
	const { data: record } = await api.get<EncryptedFileRecord>(`/api/files/${encodeURIComponent(fileId)}/record`, { signal });
	const contextId = crypto.randomUUID();
	const opened = await openFileContext(contextId, record);
	const chunks: Uint8Array[] = [];

	try {
		const manifest = JSON.parse(opened.manifest) as FileManifest;
		if (!Array.isArray(manifest.chunks) || manifest.chunks.length !== record.totalChunks) {
			throw new Error('The file manifest is invalid.');
		}

		let cipherStart = 0;
		for (const [index, chunk] of manifest.chunks.entries()) {
			if (signal.aborted) throw new DOMException('Preview canceled.', 'AbortError');
			if (!Number.isSafeInteger(chunk.cipher_size) || chunk.cipher_size <= 0) {
				throw new Error('The file manifest is invalid.');
			}

			const cipherEnd = cipherStart + chunk.cipher_size - 1;
			const response = await fetch(record.sourceUrl, {
				headers: { Range: `bytes=${cipherStart}-${cipherEnd}` },
				signal
			});
			if (response.status !== 206) throw new Error(`Could not load file data (${response.status}).`);

			const encryptedChunk = new Uint8Array(await response.arrayBuffer());
			try {
				if (encryptedChunk.byteLength !== chunk.cipher_size) throw new Error('The file data is incomplete.');
				const aad = `arkive:file-chunk:v1:${record.vaultId}:${record.fileId}:${index + 1}:${record.chunkSize}:${record.totalChunks}`;
				const { chunkBytes } = await decryptFileChunk(contextId, encryptedChunk, aad, chunk.hash);
				chunks.push(chunkBytes);
			} finally {
				encryptedChunk.fill(0);
			}
			cipherStart += chunk.cipher_size;
		}

		return new Blob(chunks as BlobPart[], { type: mimeType });
	} finally {
		for (const chunk of chunks) chunk.fill(0);
		await closeFileContext(contextId).catch(() => {});
	}
}
