import { setVaultUnlocked } from './state';
import type { VaultWorkerCalls, VaultWorkerMethod } from './types';

type WorkerResponse = { id: number; ok: true; result?: unknown } | { id: number; ok: false; error?: string };
type PendingRequest = { resolve: (value: unknown) => void; reject: (error: Error) => void };

let worker: Worker | null = null;
let requestID = 0;
const pending = new Map<number, PendingRequest>();

function getWorker(): Worker {
	if (worker) return worker;
	if (typeof Worker === 'undefined') throw new Error('Vault crypto is only available in the browser.');
	worker = new Worker(new URL('./crypto.worker.ts', import.meta.url), { type: 'module' });
	worker.addEventListener('message', (event: MessageEvent<WorkerResponse>) => {
		const response = event.data;
		const request = pending.get(response.id);
		if (!request) return;
		pending.delete(response.id);
		if (response.ok) request.resolve(response.result || {});
		else request.reject(new Error(response.error || 'Vault worker failed'));
	});
	worker.addEventListener('error', (event: ErrorEvent) => {
		for (const request of pending.values()) request.reject(new Error('Vault worker failed'));
		pending.clear();
		worker = null;
		setVaultUnlocked(false);
		event.preventDefault();
	});
	return worker;
}

export function callVaultWorker<K extends VaultWorkerMethod>(
	method: K,
	params: VaultWorkerCalls[K]['params'],
	transfer: Transferable[] = []
): Promise<VaultWorkerCalls[K]['result']> {
	return new Promise((resolve, reject) => {
		const id = ++requestID;
		pending.set(id, { resolve: (value) => resolve(value as VaultWorkerCalls[K]['result']), reject });
		try {
			getWorker().postMessage({ id, method, params }, transfer);
		} catch (error) {
			pending.delete(id);
			reject(error instanceof Error ? error : new Error('Vault worker failed'));
		}
	});
}
