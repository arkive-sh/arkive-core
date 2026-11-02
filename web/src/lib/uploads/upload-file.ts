import { CORE_API_ORIGIN } from '$app/env/public';
import { api } from '../api';
import { createSearchTokens } from '../vault/search';
import { waitUntilReady } from '../vault/session';
import { encryptFolderMetadata, encryptFolderName } from '../vault/files';
import { ENCRYPTED_ENVELOPE_OVERHEAD_BYTES, ENCRYPTION_VERSION } from '../vault/crypto-format';
import type { SearchTokenEntry } from '../vault/types';
import { createAndUploadThumbnail, type UploadedThumbnail } from './thumbnail';
import { imageMimeType } from '#lib/utils/image.ts';
import {
	clearUploadContext,
	encryptUploadMetadata,
	encryptUploadPart,
	finalizeUpload,
	hashUploadPayload,
	prepareUpload
} from '../vault/uploads';

const FILE_CHUNK_SIZE = 4 * 1024 * 1024;
const UPLOAD_PART_SIZE = 8 * 1024 * 1024;
const PRESIGN_BATCH_SIZE = 8;

interface UploadStart {
	fileId: string;
	vaultId: string;
	uploadSessionId: string;
	providerUploadId: string;
	fileChunkSize: number;
	totalChunks: number;
	uploadPartSize: number;
	uploadPartCount: number;
	uploadUrl?: string;
}

function extension(name: string): string {
	const dot = name.lastIndexOf('.');
	return dot > 0 ? name.slice(dot + 1).toLowerCase() : '';
}

function checkAborted(signal?: AbortSignal): void {
	if (signal?.aborted) throw new DOMException('Upload canceled.', 'AbortError');
}

function storageURL(url: string): string {
	return new URL(url, CORE_API_ORIGIN).toString();
}

function createPartURLGetter(started: UploadStart, signal: AbortSignal): (partNumber: number) => Promise<string> {
	const urls = new Map<number, string>();
	let nextBatchStart = 1;
	let pendingBatch: Promise<void> | null = null;

	return async (partNumber) => {
		if (started.uploadUrl) return storageURL(started.uploadUrl);
		while (!urls.has(partNumber)) {
			if (!pendingBatch) {
				const first = nextBatchStart;
				if (first > started.uploadPartCount) throw new Error('Storage did not provide an upload URL.');
				const parts = Array.from(
					{ length: Math.min(PRESIGN_BATCH_SIZE, started.uploadPartCount - first + 1) },
					(_, index) => first + index
				);
				nextBatchStart += parts.length;
				pendingBatch = api.post<{ urls: Record<string, string> }>(
					`/api/uploads/${encodeURIComponent(started.uploadSessionId)}/parts/presign`,
					{ parts },
					{ signal }
				).then(({ data }) => {
					for (const [number, url] of Object.entries(data.urls)) urls.set(Number(number), url);
					if (parts.some((number) => !urls.has(number))) throw new Error('Storage did not provide an upload URL.');
				}).finally(() => {
					pendingBatch = null;
				});
			}
			await pendingBatch;
		}
		const url = urls.get(partNumber);
		urls.delete(partNumber);
		if (!url) throw new Error('Storage did not provide an upload URL.');
		return storageURL(url);
	};
}

async function cancelUpload(uploadSessionId: string): Promise<void> {
	await api.post(`/api/uploads/${encodeURIComponent(uploadSessionId)}/cancel`).catch(() => {});
}

export async function createUploadFolder(
	name: string,
	parentFolderId: string | null,
	vaultId: string,
	signal: AbortSignal
): Promise<string> {
	const [{ encryptedMetadata: encryptedName }, { encryptedMetadata }, tokens] = await Promise.all([
		encryptFolderName({ name }),
		encryptFolderMetadata({ name }),
		createSearchTokens(name, vaultId, 128)
	]);
	if (signal.aborted) throw new DOMException('Upload canceled.', 'AbortError');
	const searchTokens = tokens.map((token): SearchTokenEntry => ({ token, field: 'name', weight: 10 }));
	const { data } = await api.post<{ folder: { id: string } }>(
		'/api/folders',
		{ parentFolderId, encryptedName, encryptedMetadata, searchTokens },
		{ signal }
	);
	return data.folder.id;
}

export async function uploadFile(
	file: File,
	signal?: AbortSignal,
	onProgress?: (uploadedBytes: number) => void,
	folderId: string | null = null,
	partConcurrency = 3
): Promise<void> {
	if (!file.size) throw new Error('Empty files cannot be uploaded.');
	if (!(await waitUntilReady()).unlocked) throw new Error('Unlock the vault before uploading files.');
	checkAborted(signal);

	const totalChunks = Math.ceil(file.size / FILE_CHUNK_SIZE);
	const encryptedSize = file.size + totalChunks * ENCRYPTED_ENVELOPE_OVERHEAD_BYTES;
	if (!Number.isSafeInteger(encryptedSize)) throw new Error('File is too large to upload.');
	const chunksPerPart = Math.floor(UPLOAD_PART_SIZE / FILE_CHUNK_SIZE);
	const uploadPartCount = Math.ceil(totalChunks / chunksPerPart);
	const uploadToken = crypto.randomUUID();
	const { data: started } = await api.post<UploadStart>(
		'/api/uploads/start',
		{
			encryptedSize,
			fileChunkSize: FILE_CHUNK_SIZE,
			totalChunks,
			uploadPartSize: UPLOAD_PART_SIZE,
			uploadPartCount,
			encryptionVersion: ENCRYPTION_VERSION,
			folderId,
			singlePart: totalChunks === 1
		},
		{ signal }
	);
	let complete = false;

	try {
		const mime = imageMimeType(file.name, file.type) || file.type || 'application/octet-stream';
		const prepared = await prepareUpload(
			uploadToken,
			started.vaultId,
			started.fileId,
			started.totalChunks
		);
		const chunksByPart: Array<Array<{ n: number; plain_size: number; cipher_size: number; hash: string }>> = new Array(started.uploadPartCount);
		let uploadedBytes = 0;
		let nextPartNumber = 1;
		const concurrency = Number.isFinite(partConcurrency)
			? Math.min(8, Math.max(1, Math.floor(partConcurrency)))
			: 3;
		const partController = new AbortController();
		const abortParts = () => partController.abort();
		signal?.addEventListener('abort', abortParts, { once: true });
		if (signal?.aborted) abortParts();
		const thumbnailPromise = createAndUploadThumbnail({
			file,
			uploadToken,
			uploadSessionId: started.uploadSessionId,
			vaultId: started.vaultId,
			fileId: started.fileId,
			signal: partController.signal
		});
		const getURL = createPartURLGetter(started, partController.signal);

		async function uploadPart(partNumber: number): Promise<void> {
			checkAborted(partController.signal);
			const partStart = (partNumber - 1) * started.uploadPartSize;
			const partEnd = Math.min(partStart + started.uploadPartSize, file.size);
			const firstChunk = Math.floor(partStart / started.fileChunkSize) + 1;
			const encryptedChunks: Uint8Array[] = [];
			const partChunks: Array<{ n: number; plain_size: number; cipher_size: number; hash: string }> = [];
			try {
				for (let offset = partStart, chunkNumber = firstChunk; offset < partEnd; offset += started.fileChunkSize, chunkNumber += 1) {
					checkAborted(partController.signal);
					const end = Math.min(offset + started.fileChunkSize, partEnd);
					const plaintext = new Uint8Array(await file.slice(offset, end).arrayBuffer());
					try {
						const encrypted = await encryptUploadPart(
							uploadToken,
							plaintext,
							`arkive:file-chunk:v1:${started.vaultId}:${started.fileId}:${chunkNumber}:${started.fileChunkSize}:${started.totalChunks}`
						);
						encryptedChunks.push(encrypted.encryptedChunk);
						partChunks.push({
							n: chunkNumber,
							plain_size: end - offset,
							cipher_size: encrypted.encryptedSize,
							hash: encrypted.encryptedHash
						});
					} finally {
						plaintext.fill(0);
					}
				}

				const partSize = encryptedChunks.reduce((size, chunk) => size + chunk.byteLength, 0);
				const body = new Uint8Array(partSize);
				try {
					let offset = 0;
					for (const chunk of encryptedChunks) {
						body.set(chunk, offset);
						offset += chunk.byteLength;
					}
					const { hash } = await hashUploadPayload(body);
					const response = await fetch(await getURL(partNumber), {
						method: 'PUT',
						headers: { 'Content-Type': 'application/octet-stream' },
						body: body.buffer,
						signal: partController.signal
					});
					if (!response.ok) throw new Error('Encrypted file data upload failed.');
					if (started.providerUploadId) {
						await api.post(
							`/api/uploads/${encodeURIComponent(started.uploadSessionId)}/parts`,
							{ partNumber, encryptedHash: hash, etag: response.headers.get('etag') || '' },
							{ signal: partController.signal }
						);
					}
					chunksByPart[partNumber - 1] = partChunks;
					uploadedBytes += partEnd - partStart;
					onProgress?.(uploadedBytes);
				} finally {
					body.fill(0);
				}
			} finally {
				for (const chunk of encryptedChunks) chunk.fill(0);
			}
		}

		const workers = Array.from({ length: Math.min(concurrency, started.uploadPartCount) }, async () => {
			while (nextPartNumber <= started.uploadPartCount) {
				checkAborted(partController.signal);
				const partNumber = nextPartNumber++;
				await uploadPart(partNumber);
			}
		});
		let uploadedThumbnail: UploadedThumbnail | null = null;
		try {
			await Promise.all(workers);
			uploadedThumbnail = await thumbnailPromise;
		} catch (error) {
			partController.abort();
			await Promise.allSettled(workers);
			throw error;
		} finally {
			signal?.removeEventListener('abort', abortParts);
		}
		const chunks = chunksByPart.flat();

		checkAborted(signal);
		const encryptedMetadata = await encryptUploadMetadata(uploadToken, {
			schema: 'arkive.file.metadata',
			version: 1,
			name: file.name,
			mime,
			extension: extension(file.name),
			size: file.size,
			dimensions: uploadedThumbnail
				? { width: uploadedThumbnail.originalWidth, height: uploadedThumbnail.originalHeight }
				: undefined,
			created_at_client: new Date().toISOString(),
			modified_at_client: file.lastModified ? new Date(file.lastModified).toISOString() : null,
			preview: uploadedThumbnail
				? {
						thumbnail_file_id: null,
						has_thumbnail: true,
						thumbnail_mime: uploadedThumbnail.mime,
						thumbnail_width: uploadedThumbnail.thumbnailWidth,
						thumbnail_height: uploadedThumbnail.thumbnailHeight,
						thumbnail_size: uploadedThumbnail.encryptedSize,
						thumbnail_version: 1
					}
				: { thumbnail_file_id: null, has_thumbnail: false }
		}, started.vaultId, started.fileId);
		const searchTokens: SearchTokenEntry[] = (await createSearchTokens(file.name, started.vaultId, 128)).map((token) => ({
			token,
			field: 'name' as const,
			weight: 10 as const
		}));
		checkAborted(signal);
		const finalized = await finalizeUpload(
			uploadToken,
			started.vaultId,
			started.fileId,
			{
				schema: 'arkive.file.manifest',
				version: 1,
				hash_alg: 'sha256',
				hash_encoding: 'base64',
				file_id: started.fileId,
				name: file.name,
				mime,
				extension: extension(file.name),
				size: file.size,
				chunk_size: started.fileChunkSize,
				chunks
			},
			chunks.map((chunk) => chunk.hash)
		);
		await api.post(
			`/api/uploads/${encodeURIComponent(started.uploadSessionId)}/complete`,
			{
				encryptedMetadata: encryptedMetadata.encryptedMetadata,
				encryptedFileKey: prepared.encryptedFileKey,
				encryptedManifest: finalized.encryptedManifest,
				encryptedHash: finalized.encryptedHash,
				searchTokens,
				hasThumbnail: !!uploadedThumbnail,
				thumbnailSize: uploadedThumbnail?.encryptedSize ?? 0
			},
			{ signal }
		);
		complete = true;
	} finally {
		if (!complete) {
			await clearUploadContext(uploadToken).catch(() => {});
			await cancelUpload(started.uploadSessionId);
		}
	}
}
