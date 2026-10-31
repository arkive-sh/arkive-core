import { api } from '#lib/api.ts';
import { closeFileContext, decryptFileChunk, openFileContext } from './files.ts';
import { waitUntilReady } from './session.ts';
import type { FileCryptoRecord, VaultMetadata } from './types.ts';

interface EncryptedFileRecord extends FileCryptoRecord {
	encryptionVersion: number;
	chunkSize: number;
	totalChunks: number;
	plaintextSize: number;
	encryptedHash: string;
	sourceUrl: string;
}

interface ManifestChunk {
	plain_size: number;
	cipher_size: number;
	hash: string;
}

interface FileManifest {
	chunks: ManifestChunk[];
}

interface FileReadContext {
	record: EncryptedFileRecord;
	metadata: VaultMetadata & { name?: string; mime?: string; size?: number };
	plaintextSize: number;
}

export class DecryptedFileReader {
	private readonly contextId = crypto.randomUUID();
	private readonly chunkCache = new Map<number, Uint8Array>();
	private readonly maxChunkCacheBytes = 32 * 1024 * 1024;
	private chunkCacheBytes = 0;
	private loadPromise?: Promise<FileReadContext>;
	private contextOpened = false;
	private disposed = false;
	private manifest?: FileManifest;
	private cipherStarts: number[] = [];

	constructor(
		private readonly fileId: string,
		private readonly signal?: AbortSignal
	) {}

	load(): Promise<FileReadContext> {
		if (this.disposed) return Promise.reject(new Error('File reader is closed.'));
		this.loadPromise ??= this.loadFile();
		return this.loadPromise;
	}

	private async loadFile(): Promise<FileReadContext> {
		const session = await waitUntilReady();
		if (!session.unlocked) throw new Error('Unlock the vault to open this file.');

		const { data: record } = await api.get<EncryptedFileRecord>(`/api/files/${encodeURIComponent(this.fileId)}/record`, {
			signal: this.signal
		});
		if (!record.sourceUrl || !Number.isSafeInteger(record.chunkSize) || record.chunkSize <= 0 || !Number.isSafeInteger(record.totalChunks) || record.totalChunks < 0) {
			throw new Error('The file record is invalid.');
		}

		const opened = await openFileContext(this.contextId, record);
		this.contextOpened = true;
		try {
			if (this.disposed) throw new Error('File reader is closed.');
			const metadata = JSON.parse(opened.metadata) as FileReadContext['metadata'];
			const manifest = JSON.parse(opened.manifest) as FileManifest;
			const plaintextSize = typeof metadata.size === 'number' ? metadata.size : record.plaintextSize;
			if (!Number.isSafeInteger(plaintextSize) || plaintextSize < 0 || !Array.isArray(manifest.chunks) || manifest.chunks.length !== record.totalChunks) {
				throw new Error('The file manifest is invalid.');
			}

			let plainOffset = 0;
			let cipherOffset = 0;
			for (const [index, chunk] of manifest.chunks.entries()) {
				const expectedPlainSize = Math.min(record.chunkSize, plaintextSize - plainOffset);
				if (
					!Number.isSafeInteger(chunk.plain_size) || chunk.plain_size !== expectedPlainSize ||
					!Number.isSafeInteger(chunk.cipher_size) || chunk.cipher_size <= 0 ||
					typeof chunk.hash !== 'string' || !chunk.hash
				) throw new Error('The file manifest is invalid.');
				this.cipherStarts[index] = cipherOffset;
				plainOffset += chunk.plain_size;
				cipherOffset += chunk.cipher_size;
			}
			if (plainOffset !== plaintextSize) throw new Error('The file manifest is invalid.');

			this.manifest = manifest;
			return { record, metadata, plaintextSize };
		} catch (error) {
			await this.closeContext();
			throw error;
		}
	}

	async readRange(start: number, end: number): Promise<Uint8Array> {
		const { record, plaintextSize } = await this.load();
		if (!Number.isSafeInteger(start) || !Number.isSafeInteger(end) || start < 0 || end < start || end > plaintextSize) {
			throw new RangeError('File range is outside its contents.');
		}
		if (start === end) return new Uint8Array(0);

		const output = new Uint8Array(end - start);
		const firstChunk = Math.floor(start / record.chunkSize);
		const lastChunk = Math.floor((end - 1) / record.chunkSize);
		let outputOffset = 0;
		try {
			for (let index = firstChunk; index <= lastChunk; index++) {
				const chunk = await this.readChunk(index);
				const chunkStart = index * record.chunkSize;
				const from = Math.max(0, start - chunkStart);
				const to = Math.min(chunk.byteLength, end - chunkStart);
				output.set(chunk.subarray(from, to), outputOffset);
				outputOffset += to - from;
			}
			if (outputOffset !== output.byteLength) throw new Error('The file data is incomplete.');
			return output;
		} catch (error) {
			output.fill(0);
			throw error;
		}
	}

	async createBlob(mimeType?: string): Promise<Blob> {
		const { plaintextSize, metadata } = await this.load();
		const parts: Uint8Array[] = [];
		try {
			for (let start = 0; start < plaintextSize; start += 4 * 1024 * 1024) {
				parts.push(await this.readRange(start, Math.min(plaintextSize, start + 4 * 1024 * 1024)));
			}
			return new Blob(parts as BlobPart[], { type: mimeType || String(metadata.mime || 'application/octet-stream') });
		} finally {
			for (const part of parts) part.fill(0);
		}
	}

	async dispose(): Promise<void> {
		if (this.disposed) return;
		this.disposed = true;
		for (const chunk of this.chunkCache.values()) chunk.fill(0);
		this.chunkCache.clear();
		this.chunkCacheBytes = 0;
		await this.closeContext();
	}

	private async readChunk(index: number): Promise<Uint8Array> {
		const context = await this.load();
		const cached = this.chunkCache.get(index);
		if (cached) {
			this.chunkCache.delete(index);
			this.chunkCache.set(index, cached);
			return cached;
		}

		const chunk = this.manifest?.chunks[index];
		if (!chunk) throw new RangeError('File chunk is outside its contents.');
		const cipherStart = this.cipherStarts[index];
		const response = await fetch(context.record.sourceUrl, {
			headers: { Range: `bytes=${cipherStart}-${cipherStart + chunk.cipher_size - 1}` },
			signal: this.signal
		});
		if (response.status !== 206) throw new Error(`Could not load file data (${response.status}).`);

		const encryptedChunk = new Uint8Array(await response.arrayBuffer());
		try {
			if (this.disposed) throw new Error('File reader is closed.');
			if (encryptedChunk.byteLength !== chunk.cipher_size) throw new Error('The file data is incomplete.');
			const aad = `arkive:file-chunk:v1:${context.record.vaultId}:${context.record.fileId}:${index + 1}:${context.record.chunkSize}:${context.record.totalChunks}`;
			const { chunkBytes } = await decryptFileChunk(this.contextId, encryptedChunk, aad, chunk.hash);
			if (this.disposed) {
				chunkBytes.fill(0);
				throw new Error('File reader is closed.');
			}
			this.storeChunk(index, chunkBytes);
			return chunkBytes;
		} finally {
			encryptedChunk.fill(0);
		}
	}

	private storeChunk(index: number, bytes: Uint8Array): void {
		const previous = this.chunkCache.get(index);
		if (previous) this.chunkCacheBytes -= previous.byteLength;
		this.chunkCache.delete(index);
		this.chunkCache.set(index, bytes);
		this.chunkCacheBytes += bytes.byteLength;
		while (this.chunkCacheBytes > this.maxChunkCacheBytes && this.chunkCache.size > 1) {
			const oldestIndex = this.chunkCache.keys().next().value;
			if (oldestIndex === undefined) break;
			const oldest = this.chunkCache.get(oldestIndex);
			oldest?.fill(0);
			this.chunkCache.delete(oldestIndex);
			this.chunkCacheBytes -= oldest?.byteLength ?? 0;
		}
	}

	private async closeContext(): Promise<void> {
		if (!this.contextOpened) return;
		this.contextOpened = false;
		await closeFileContext(this.contextId).catch(() => {});
	}
}

export async function readDecryptedFile(fileId: string, mimeType: string, signal: AbortSignal): Promise<Blob> {
	const reader = new DecryptedFileReader(fileId, signal);
	try {
		return await reader.createBlob(mimeType);
	} finally {
		await reader.dispose();
	}
}
