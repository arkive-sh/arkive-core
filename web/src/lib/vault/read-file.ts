import { api } from '#lib/api.ts';
import { decodeJSON } from './bytes.ts';
import { closeFileContext, decryptFileChunk, openFileContext } from './files.ts';
import { waitUntilReady } from './session.ts';
import type { FileCryptoRecord, VaultMetadata } from './types.ts';
import { ENCRYPTED_ENVELOPE_OVERHEAD_BYTES, ENCRYPTION_VERSION } from './crypto-format.ts';

interface EncryptedFileRecord extends FileCryptoRecord {
	encryptionVersion: number;
	chunkSize: number;
	totalChunks: number;
	encryptedHash: string;
	sourceUrl: string;
}

interface ManifestChunk {
	plain_size: number;
	cipher_size: number;
	hash: string;
}

interface FileManifest {
	hash_alg: 'sha256';
	chunks: ManifestChunk[];
}

interface FileReadContext {
	record: EncryptedFileRecord;
	metadata: VaultMetadata & { name?: string; mime?: string; size?: number };
	plaintextSize: number;
}

interface PendingChunkRead {
	controller: AbortController;
	consumers: number;
	settled: boolean;
	promise: Promise<Uint8Array>;
}

function clearMetadata(metadata?: VaultMetadata): void {
	if (metadata) for (const key of Object.keys(metadata)) delete metadata[key];
}

function clearManifest(manifest?: FileManifest): void {
	if (!manifest || !Array.isArray(manifest.chunks)) return;
	for (const chunk of manifest.chunks) {
		if (!chunk || typeof chunk !== 'object') continue;
		chunk.plain_size = 0;
		chunk.cipher_size = 0;
		chunk.hash = '';
	}
	manifest.chunks.length = 0;
}

export class DecryptedFileReader {
	private readonly contextId = crypto.randomUUID();

	private readonly chunkCache = new Map<number, Uint8Array>();

	private readonly pendingChunks = new Map<number, PendingChunkRead>();

	private readonly maxChunkCacheBytes = 32 * 1024 * 1024;

	private chunkCacheBytes = 0;

	private loadPromise?: Promise<FileReadContext>;

	private contextOpened = false;

	private disposed = false;

	private manifest?: FileManifest;

	private metadata?: FileReadContext['metadata'];

	private cipherStarts: number[] = [];

	constructor(
		private readonly fileId: string,
		private readonly signal?: AbortSignal,
	) {}

	load(): Promise<FileReadContext> {
		if (this.disposed) return Promise.reject(new Error('File reader is closed.'));
		this.loadPromise ??= this.loadFile();

		return this.loadPromise;
	}

	private async loadFile(): Promise<FileReadContext> {
		const session = await waitUntilReady();

		if (!session.unlocked) throw new Error('Unlock the vault to open this file.');

		const { data: record } = await api.get<EncryptedFileRecord>(
			`/api/files/${encodeURIComponent(this.fileId)}/record`,
			{
				signal: this.signal,
			},
		);

		if (
			record.encryptionVersion !== ENCRYPTION_VERSION ||
			!record.sourceUrl ||
			!Number.isSafeInteger(record.chunkSize) ||
			record.chunkSize <= 0 ||
			!Number.isSafeInteger(record.totalChunks) ||
			record.totalChunks < 0
		) {
			throw new Error('The file record is invalid.');
		}

		const opened = await openFileContext(this.contextId, record);
		this.contextOpened = true;
		let metadataBytes: Uint8Array | null = opened.metadataBytes;
		let manifestBytes: Uint8Array | null = opened.manifestBytes;
		let metadata: FileReadContext['metadata'] | undefined;
		let manifest: FileManifest | undefined;

		try {
			if (this.disposed) throw new Error('File reader is closed.');
			if (!metadataBytes || !manifestBytes) throw new Error('The file context is incomplete.');
			metadata = decodeJSON<FileReadContext['metadata']>(metadataBytes);
			metadataBytes = null;
			manifest = decodeJSON<FileManifest>(manifestBytes);
			manifestBytes = null;
			const plaintextSize = metadata.size;

			// Reject malformed authenticated manifests before they can request oversized ciphertext ranges.
			if (
				manifest.hash_alg !== 'sha256' ||
				typeof plaintextSize !== 'number' ||
				!Number.isSafeInteger(plaintextSize) ||
				plaintextSize < 0 ||
				record.totalChunks !== Math.ceil(plaintextSize / record.chunkSize) ||
				!Array.isArray(manifest.chunks) ||
				manifest.chunks.length !== record.totalChunks
			) {
				throw new Error('The file manifest is invalid.');
			}

			let plainOffset = 0;
			let cipherOffset = 0;

			for (const [index, chunk] of manifest.chunks.entries()) {
				if (!chunk || typeof chunk !== 'object') throw new Error('The file manifest is invalid.');
				const expectedPlainSize = Math.min(record.chunkSize, plaintextSize - plainOffset);

				if (
					!Number.isSafeInteger(chunk.plain_size) ||
					chunk.plain_size !== expectedPlainSize ||
					!Number.isSafeInteger(chunk.cipher_size) ||
					chunk.cipher_size !== chunk.plain_size + ENCRYPTED_ENVELOPE_OVERHEAD_BYTES ||
					typeof chunk.hash !== 'string' ||
					!/^[A-Za-z0-9+/]{43}=$/.test(chunk.hash)
				)
					throw new Error('The file manifest is invalid.');
				const nextCipherOffset = cipherOffset + chunk.cipher_size;

				if (!Number.isSafeInteger(nextCipherOffset))
					throw new Error('The file manifest is invalid.');
				this.cipherStarts[index] = cipherOffset;
				plainOffset += chunk.plain_size;
				cipherOffset = nextCipherOffset;
			}
			if (plainOffset !== plaintextSize) throw new Error('The file manifest is invalid.');

			this.manifest = manifest;
			this.metadata = metadata;

			return { record, metadata, plaintextSize };
		} catch (error) {
			await this.closeContext();

			throw error;
		} finally {
			metadataBytes?.fill(0);
			metadataBytes = null;
			manifestBytes?.fill(0);
			manifestBytes = null;
			if (metadata !== this.metadata) clearMetadata(metadata);
			if (manifest !== this.manifest) clearManifest(manifest);
		}
	}

	async readRange(start: number, end: number, requestSignal?: AbortSignal): Promise<Uint8Array> {
		// A canceled media range and closing the viewer must both stop the same fetch.
		const signal =
			this.signal && requestSignal
				? AbortSignal.any([this.signal, requestSignal])
				: (requestSignal ?? this.signal);
		signal?.throwIfAborted();
		const { record, plaintextSize } = await this.load();
		signal?.throwIfAborted();
		if (
			!Number.isSafeInteger(start) ||
			!Number.isSafeInteger(end) ||
			start < 0 ||
			end < start ||
			end > plaintextSize
		) {
			throw new RangeError('File range is outside its contents.');
		}
		if (start === end) return new Uint8Array(0);

		const output = new Uint8Array(end - start);
		const firstChunk = Math.floor(start / record.chunkSize);
		const lastChunk = Math.floor((end - 1) / record.chunkSize);
		let outputOffset = 0;

		try {
			for (let index = firstChunk; index <= lastChunk; index++) {
				const chunk = await this.readChunk(index, signal);
				signal?.throwIfAborted();
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

			return new Blob(parts as BlobPart[], {
				type: mimeType || String(metadata.mime || 'application/octet-stream'),
			});
		} finally {
			for (const part of parts) part.fill(0);
		}
	}

	async dispose(): Promise<void> {
		if (this.disposed) return;
		this.disposed = true;

		for (const pending of this.pendingChunks.values()) pending.controller.abort();
		this.pendingChunks.clear();

		for (const chunk of this.chunkCache.values()) chunk.fill(0);
		this.chunkCache.clear();
		this.chunkCacheBytes = 0;
		clearMetadata(this.metadata);
		this.metadata = undefined;
		clearManifest(this.manifest);
		this.manifest = undefined;
		this.cipherStarts.fill(0);
		this.cipherStarts.length = 0;
		this.loadPromise = undefined;

		await this.closeContext();
	}

	private async readChunk(index: number, signal?: AbortSignal): Promise<Uint8Array> {
		signal?.throwIfAborted();
		const context = await this.load();
		signal?.throwIfAborted();
		if (this.disposed) throw new Error('File reader is closed.');
		const cached = this.chunkCache.get(index);

		if (cached) {
			this.chunkCache.delete(index);
			this.chunkCache.set(index, cached);

			return cached;
		}

		// Share fetch/decrypt work, but abort it only after every range stops waiting.
		let pending = this.pendingChunks.get(index);

		if (!pending) {
			const controller = new AbortController();
			const readSignal = this.signal
				? AbortSignal.any([this.signal, controller.signal])
				: controller.signal;
			pending = {
				controller,
				consumers: 0,
				settled: false,
				promise: Promise.resolve().then(() => this.loadChunk(index, context, readSignal)),
			};
			const current = pending;
			pending.promise = pending.promise.finally(() => {
				current.settled = true;
				if (this.pendingChunks.get(index) === current) this.pendingChunks.delete(index);
			});
			this.pendingChunks.set(index, pending);
		}

		pending.consumers++;
		let onAbort: (() => void) | undefined;

		try {
			if (!signal) return await pending.promise;
			const canceled = new Promise<Uint8Array>((_, reject) => {
				onAbort = () =>
					reject(signal.reason ?? new DOMException('Stream read was cancelled.', 'AbortError'));
				signal.addEventListener('abort', onAbort, { once: true });
				signal.throwIfAborted();
			});

			return await Promise.race([pending.promise, canceled]);
		} finally {
			if (onAbort) signal?.removeEventListener('abort', onAbort);
			pending.consumers--;
			if (!pending.consumers && !pending.settled) {
				if (this.pendingChunks.get(index) === pending) this.pendingChunks.delete(index);
				pending.controller.abort();
			}
		}
	}

	private async loadChunk(
		index: number,
		context: FileReadContext,
		signal: AbortSignal,
	): Promise<Uint8Array> {
		signal.throwIfAborted();
		const chunk = this.manifest?.chunks[index];

		if (!chunk) throw new RangeError('File chunk is outside its contents.');

		const cipherStart = this.cipherStarts[index];
		const response = await fetch(context.record.sourceUrl, {
			headers: { Range: `bytes=${cipherStart}-${cipherStart + chunk.cipher_size - 1}` },
			signal,
		});

		if (response.status !== 206) throw new Error(`Could not load file data (${response.status}).`);

		const encryptedChunk = new Uint8Array(await response.arrayBuffer());

		try {
			signal.throwIfAborted();
			if (this.disposed) throw new Error('File reader is closed.');
			if (encryptedChunk.byteLength !== chunk.cipher_size)
				throw new Error('The file data is incomplete.');

			const aad = `arkive:file-chunk:v1:${context.record.vaultId}:${context.record.fileId}:${index + 1}:${context.record.chunkSize}:${context.record.totalChunks}`;

			const { chunkBytes } = await decryptFileChunk(
				this.contextId,
				encryptedChunk,
				aad,
				chunk.hash,
			);

			if (this.disposed || signal.aborted) {
				chunkBytes.fill(0);
				if (signal.aborted) signal.throwIfAborted();

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

export async function readDecryptedFile(
	fileId: string,
	mimeType: string,
	signal: AbortSignal,
): Promise<Blob> {
	const reader = new DecryptedFileReader(fileId, signal);

	try {
		return await reader.createBlob(mimeType);
	} finally {
		await reader.dispose();
	}
}
