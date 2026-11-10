import { lockVault } from './crypto-worker/runtime';
import { file_operations } from './crypto-worker/file-operations';
import { search_operations } from './crypto-worker/search-operations';
import { share_operations } from './crypto-worker/share-operations';
import { upload_operations } from './crypto-worker/upload-operations';
import { vault_operations } from './crypto-worker/vault-operations';
import { workerState } from './crypto-worker/runtime';
import type {
	WorkerHandler,
	WorkerHandlers,
	WorkerInput,
	WorkerResult,
} from './crypto-worker/types';
import type { VaultWorkerMethod } from './types';

type WorkerRequest = {
	[K in VaultWorkerMethod]: { id: number; method: K; params: WorkerInput<K> };
}[VaultWorkerMethod];
type WorkerResponse =
	{ id: number; ok: true; result: WorkerResult } | { id: number; ok: false; error: string };

const messageHandlers: WorkerHandlers = {
	...vault_operations,
	...share_operations,
	...upload_operations,
	...file_operations,
	...search_operations,
};

async function handleMessage(message: WorkerRequest): Promise<WorkerResult> {
	// Map lookup loses the method/params correlation; WorkerRequest preserves it.
	const handler = messageHandlers[message.method] as WorkerHandler<VaultWorkerMethod>;

	return handler(message.params);
}

const workerScope = self as unknown as {
	addEventListener(type: 'message', listener: (event: MessageEvent<WorkerRequest>) => void): void;
	postMessage(message: WorkerResponse, transfer?: Transferable[]): void;
};

function clearWorkerResult(result: WorkerResult): void {
	// Results can hold plaintext byte arrays or recovery-key text that must be dropped on lock.
	for (const [key, value] of Object.entries(result)) {
		if (value instanceof Uint8Array) value.fill(0);
		else if (Array.isArray(value)) value.fill('');
		result[key] = null;
	}
}

workerScope.addEventListener('message', async (event) => {
	const payload = event.data;

	// WebCrypto is asynchronous; discard outputs from work that finishes after a lock.
	if (payload.method === 'unlockVault') lockVault();
	const generation = workerState.vaultGeneration;
	let result: WorkerResult | undefined;

	try {
		result = await handleMessage(payload);
		if (payload.method !== 'lockVault' && generation !== workerState.vaultGeneration) {
			clearWorkerResult(result);
			result = undefined;

			throw new Error('Vault was locked during this operation.');
		}
		const transfer: Transferable[] = [];

		for (const value of [
			result.masterKey,
			result.shareSecret,
			result.encryptedChunk,
			result.encryptedThumbnail,
			result.chunkBytes,
			result.thumbnailBytes,
			result.metadataBytes,
			result.manifestBytes,
			result.nameBytes,
		]) {
			if (value instanceof Uint8Array && value.buffer instanceof ArrayBuffer)
				transfer.push(value.buffer);
		}
		workerScope.postMessage({ id: payload.id, ok: true, result }, transfer);
		result = undefined;
	} catch (error) {
		if (result) clearWorkerResult(result);
		workerScope.postMessage({
			id: payload.id,
			ok: false,
			error: error instanceof Error ? error.message : 'Vault worker failed',
		});
	}
});
