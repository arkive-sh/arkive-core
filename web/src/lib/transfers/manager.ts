import { getContext, setContext } from 'svelte';
import { get, writable, type Readable } from 'svelte/store';

export type TransferKind = 'upload' | 'download';
export type TransferStatus = 'queued' | 'running' | 'completed' | 'failed' | 'canceled';

export interface Transfer {
	id: string;
	kind: TransferKind;
	fileName: string;
	totalBytes: number;
	transferredBytes: number;
	status: TransferStatus;
	error?: string;
}

export interface TransferState {
	jobs: Transfer[];
	open: boolean;
}

export type TransferRunner = (signal: AbortSignal, onProgress: (transferredBytes: number) => void) => Promise<void>;

export interface TransferManager {
	state: Readable<TransferState>;
	enqueueUploads(files: File[]): number;
	enqueueDownload(fileName: string, totalBytes: number, run: TransferRunner): string;
	setOpen(open: boolean): void;
	cancel(id: string): void;
	cancelAll(): Promise<void>;
	cancelUploads(): Promise<void>;
	retry(id: string): void;
	remove(id: string): void;
	clearCompleted(): void;
}

const contextKey = Symbol('arkive-transfer-manager');

export function provideTransferManager(manager: TransferManager): void {
	setContext(contextKey, manager);
}

export function useTransferManager(): TransferManager {
	const manager = getContext<TransferManager | undefined>(contextKey);
	if (!manager) throw new Error('Transfer manager is unavailable.');
	return manager;
}

export function createTransferManager(): TransferManager {
	// shortcut: transfers live for this tab, persist task data and server checkpoints before adding reload recovery.
	const state = writable<TransferState>({ jobs: [], open: false });
	const runners = new Map<string, TransferRunner>();
	const controllers = new Map<string, AbortController>();
	const runningTasks = new Map<string, Promise<void>>();
	let processing = false;

	function updateJob(id: string, update: Partial<Transfer>): void {
		state.update((current) => ({
			...current,
			jobs: current.jobs.map((job) => job.id === id ? { ...job, ...update } : job)
		}));
	}

	async function runJob(job: Transfer, controller: AbortController): Promise<void> {
		const run = runners.get(job.id);
		if (!run) {
			updateJob(job.id, { status: 'failed', error: 'This transfer cannot be retried.' });
			return;
		}

		updateJob(job.id, { status: 'running', error: undefined });
		try {
			await run(controller.signal, (transferredBytes) => updateJob(job.id, {
				transferredBytes: Math.min(job.totalBytes, Math.max(0, transferredBytes))
			}));
			updateJob(job.id, { status: 'completed', transferredBytes: job.totalBytes });
      runners.delete(job.id);

      if (job.kind === 'upload' && typeof window !== 'undefined') window.dispatchEvent(new Event('arkive:upload-complete'));

		} catch (error) {
			if (controller.signal.aborted || (error instanceof Error && error.name === 'AbortError')) {
				updateJob(job.id, { status: 'canceled' });
			} else {
				updateJob(job.id, {
					status: 'failed',
					error: error instanceof Error ? error.message : 'Transfer failed.'
				});
			}
		} finally {
			controllers.delete(job.id);
		}
	}

	async function runQueue(): Promise<void> {
		if (processing || typeof window === 'undefined') return;
		processing = true;
		try {
			while (true) {
				const next = get(state).jobs.find((job) => job.status === 'queued');
				if (!next) return;
				const controller = new AbortController();
				controllers.set(next.id, controller);
				const task = runJob(next, controller);
				runningTasks.set(next.id, task);
				await task;
				runningTasks.delete(next.id);
			}
		} finally {
			processing = false;
		}
	}

	function enqueueDownload(fileName: string, totalBytes: number, run: TransferRunner): string {
		if (typeof window === 'undefined') return '';
		const id = crypto.randomUUID();
		runners.set(id, run);
		state.update((current) => ({
			...current,
			jobs: [...current.jobs, { id, kind: 'download', fileName, totalBytes, transferredBytes: 0, status: 'queued' }],
			open: true
		}));
		void runQueue();
		return id;
	}

	function cancel(id: string): void {
		const job = get(state).jobs.find((entry) => entry.id === id);
		if (!job || job.status === 'completed' || job.status === 'failed' || job.status === 'canceled') return;
		if (job.status === 'queued') updateJob(id, { status: 'canceled' });
		else controllers.get(id)?.abort();
	}

	async function cancelAll(): Promise<void> {
		const jobs = get(state).jobs;
		for (const job of jobs) {
			if (job.status === 'queued') updateJob(job.id, { status: 'canceled' });
			else if (job.status === 'running') controllers.get(job.id)?.abort();
		}
		await Promise.all(jobs.filter((job) => job.status === 'running').map((job) => runningTasks.get(job.id)).filter((task): task is Promise<void> => !!task));
	}

	async function cancelUploads(): Promise<void> {
		const jobs = get(state).jobs;
		for (const job of jobs) {
			if (job.kind !== 'upload') continue;
			if (job.status === 'queued') updateJob(job.id, { status: 'canceled' });
			else if (job.status === 'running') controllers.get(job.id)?.abort();
		}
		await Promise.all(jobs.filter((job) => job.kind === 'upload').map((job) => runningTasks.get(job.id)).filter((task): task is Promise<void> => !!task));
		for (const job of get(state).jobs) {
			if (job.kind === 'upload') runners.delete(job.id);
		}
		state.update((current) => ({ ...current, jobs: current.jobs.filter((job) => job.kind !== 'upload') }));
	}

	function retry(id: string): void {
		const job = get(state).jobs.find((entry) => entry.id === id);
		if (!job || !runners.has(id) || (job.status !== 'failed' && job.status !== 'canceled')) return;
		updateJob(id, { status: 'queued', transferredBytes: 0, error: undefined });
		void runQueue();
	}

	return {
		state,
		enqueueUploads(files) {
			if (typeof window === 'undefined' || files.length === 0) return 0;
			const jobs = files.map((file): Transfer => {
				const id = crypto.randomUUID();
				runners.set(id, async (signal, onProgress) => {
					const { uploadFile } = await import('#lib/uploads/upload-file.ts');
					await uploadFile(file, signal, onProgress);
				});
				return { id, kind: 'upload', fileName: file.name, totalBytes: file.size, transferredBytes: 0, status: 'queued' };
			});
			state.update((current) => ({ ...current, jobs: [...current.jobs, ...jobs], open: true }));
			void runQueue();
			return jobs.length;
		},
		enqueueDownload,
		setOpen(open) {
			state.update((current) => ({ ...current, open }));
		},
		cancel,
		cancelAll,
		cancelUploads,
		retry,
		remove(id) {
			state.update((current) => ({
				...current,
				jobs: current.jobs.filter((job) => job.id !== id || job.status === 'queued' || job.status === 'running')
			}));
			if (!get(state).jobs.some((job) => job.id === id)) runners.delete(id);
		},
		clearCompleted() {
			state.update((current) => ({
				...current,
				jobs: current.jobs.filter((job) => job.status !== 'completed')
			}));
		}
	};
}
