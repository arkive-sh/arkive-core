import type { VaultWorkerCalls, VaultWorkerMethod } from '../types';

export type WorkerInput<K extends VaultWorkerMethod> = VaultWorkerCalls[K]['params'];
export interface WorkerResult {
	[key: string]: unknown;
	encryptedChunk?: unknown;
	encryptedThumbnail?: unknown;
	chunkBytes?: unknown;
	thumbnailBytes?: unknown;
}
export type WorkerHandler<K extends VaultWorkerMethod> = (
	params: WorkerInput<K>,
) => WorkerResult | Promise<WorkerResult>;
export type WorkerHandlers = { [K in VaultWorkerMethod]: WorkerHandler<K> };
