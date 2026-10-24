import { CORE_API_ORIGIN } from '$app/env/public';
import { api } from '../api';
import { createSearchTokens } from '../vault/search';
import { waitUntilReady } from '../vault/session';
import { encryptFolderMetadata, encryptFolderName } from '../vault/files';
import type { SearchTokenEntry } from '../vault/types';
import {
	clearUploadContext,
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

async function getPartURL(started: UploadStart, partNumber: number, signal?: AbortSignal): Promise<string> {
	if (started.uploadUrl) return storageURL(started.uploadUrl);
	const parts = Array.from(
		{ length: Math.min(PRESIGN_BATCH_SIZE, started.uploadPartCount - partNumber + 1) },
		(_, index) => partNumber + index
	);
	const { data: result } = await api.post<{ urls: Record<string, string> }>(
		`/api/uploads/${encodeURIComponent(started.uploadSessionId)}/parts/presign`,
		{ parts },
		{ signal }
	);
	const url = result.urls[String(partNumber)];
	if (!url) throw new Error('Storage did not provide an upload URL.');
	return storageURL(url);
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
	folderId: string | null = null
): Promise<void> {
	if (!file.size) throw new Error('Empty files cannot be uploaded.');
	if (!(await waitUntilReady()).unlocked) throw new Error('Unlock the vault before uploading files.');
	checkAborted(signal);

	const totalChunks = Math.ceil(file.size / FILE_CHUNK_SIZE);
	const chunksPerPart = Math.floor(UPLOAD_PART_SIZE / FILE_CHUNK_SIZE);
	const uploadPartCount = Math.ceil(totalChunks / chunksPerPart);
	const uploadToken = crypto.randomUUID();
	const { data: started } = await api.post<UploadStart>(
		'/api/uploads/start',
		{
			originalSize: file.size,
			fileChunkSize: FILE_CHUNK_SIZE,
			totalChunks,
			uploadPartSize: UPLOAD_PART_SIZE,
			uploadPartCount,
			encryptionVersion: 1,
			folderId,
			singlePart: totalChunks === 1
		},
		{ signal }
	);
	let complete = false;

	try {
		const mime = file.type || 'application/octet-stream';
		const createdAt = new Date().toISOString();
		const prepared = await prepareUpload(
			uploadToken,
			started.vaultId,
			started.fileId,
			{
				schema: 'arkive.file.metadata',
				version: 1,
				name: file.name,
				mime,
				extension: extension(file.name),
				size: file.size,
				created_at_client: createdAt,
				modified_at_client: file.lastModified ? new Date(file.lastModified).toISOString() : null,
				preview: { thumbnail_file_id: null, has_thumbnail: false }
			},
			started.totalChunks
		);
		const chunks: Array<{ n: number; plain_size: number; cipher_size: number; hash: string }> = [];

		for (let partNumber = 1; partNumber <= started.uploadPartCount; partNumber += 1) {
			checkAborted(signal);
			const partStart = (partNumber - 1) * started.uploadPartSize;
			const partEnd = Math.min(partStart + started.uploadPartSize, file.size);
			const firstChunk = Math.floor(partStart / started.fileChunkSize) + 1;
			const encryptedChunks: Uint8Array[] = [];
			try {
				for (let offset = partStart, chunkNumber = firstChunk; offset < partEnd; offset += started.fileChunkSize, chunkNumber += 1) {
					checkAborted(signal);
					const end = Math.min(offset + started.fileChunkSize, partEnd);
					const plaintext = new Uint8Array(await file.slice(offset, end).arrayBuffer());
					try {
						const encrypted = await encryptUploadPart(
							uploadToken,
							plaintext,
							`arkive:file-chunk:v1:${started.vaultId}:${started.fileId}:${chunkNumber}:${started.fileChunkSize}:${started.totalChunks}`
						);
						encryptedChunks.push(encrypted.encryptedChunk);
						chunks.push({
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
					const response = await fetch(await getPartURL(started, partNumber, signal), {
						method: 'PUT',
						headers: { 'Content-Type': 'application/octet-stream' },
						body: body.buffer,
						signal
					});
					if (!response.ok) throw new Error('Encrypted file data upload failed.');
					if (started.providerUploadId) {
						await api.post(
							`/api/uploads/${encodeURIComponent(started.uploadSessionId)}/parts`,
							{ partNumber, encryptedHash: hash, etag: response.headers.get('etag') || '' },
							{ signal }
						);
					}
				onProgress?.(partEnd);
				} finally {
					body.fill(0);
				}
			} finally {
				for (const chunk of encryptedChunks) chunk.fill(0);
			}
		}

		checkAborted(signal);
		const finalized = await finalizeUpload(
			uploadToken,
			started.vaultId,
			started.fileId,
			{
				schema: 'arkive.file.manifest',
				version: 1,
				hash_alg: 'blake3',
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
		const searchTokens: SearchTokenEntry[] = (await createSearchTokens(file.name, started.vaultId, 128)).map((token) => ({
			token,
			field: 'name' as const,
			weight: 10 as const
		}));
		checkAborted(signal);
		await api.post(
			`/api/uploads/${encodeURIComponent(started.uploadSessionId)}/complete`,
			{
				encryptedMetadata: prepared.encryptedMetadata,
				encryptedFileKey: prepared.encryptedFileKey,
				encryptedManifest: finalized.encryptedManifest,
				encryptedHash: finalized.encryptedHash,
				searchTokens
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
