import { getContext, setContext } from 'svelte';
import { get, writable, type Readable } from 'svelte/store';
import { UPLOAD_PART_SIZE } from '#lib/uploads/constants.ts';

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

export interface UploadItem {
	file: UploadSource;
	relativePath: string;
}

export interface UploadSource {
	name: string;
	type: string;
	size: number;
	lastModified: number;
	slice(start: number, end: number): Pick<Blob, 'arrayBuffer'>;
	dispose?(): void;
}

export type TransferRunner = (
	signal: AbortSignal,
	onProgress: (transferredBytes: number) => void,
	concurrency: number,
) => Promise<void>;

export interface TransferManager {
	state: Readable<TransferState>;
	enqueueUploads(files: UploadItem[], vaultId: string, parentFolderId: string | null): number;
	enqueueDownload(fileName: string, totalBytes: number, run: TransferRunner): string;
	setOpen(open: boolean): void;
	cancelMany(ids: string[]): void;
	cancelAll(): Promise<void>;
	cancelUploads(): Promise<void>;
	retryMany(ids: string[]): void;
	removeMany(ids: string[]): void;
	clearCompleted(): void;
}

const contextKey = Symbol('arkive-transfer-manager');
const DEVICE_CORES = typeof navigator === 'undefined' ? 6 : navigator.hardwareConcurrency || 6;
const TRANSFER_CHUNK_CONCURRENCY = Math.min(4, Math.max(3, Math.ceil(DEVICE_CORES / 2)));
const MAX_CONCURRENT_UPLOAD_FILES = Math.min(6, Math.max(2, DEVICE_CORES));
const MAX_CONCURRENT_LARGE_UPLOAD_FILES = 4;
const SMALL_UPLOAD_MAX_BYTES = UPLOAD_PART_SIZE;

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
	const uploadSources = new Map<string, UploadSource>();
	const controllers = new Map<string, AbortController>();
	const runningTasks = new Map<string, Promise<void>>();
	let processing = false;
	let uploadsChanged = false;

	function notifyUploadsChangedIfIdle(): void {
		if (!uploadsChanged || typeof window === 'undefined') return;
		if (
			get(state).jobs.some(
				(job) => job.kind === 'upload' && (job.status === 'queued' || job.status === 'running'),
			)
		)
			return;
		uploadsChanged = false;
		window.dispatchEvent(new Event('arkive:upload-complete'));
	}

	function updateJob(id: string, update: Partial<Transfer>): void {
		state.update((current) => ({
			...current,
			jobs: current.jobs.map((job) => (job.id === id ? { ...job, ...update } : job)),
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
			await run(
				controller.signal,
				(transferredBytes) =>
					updateJob(job.id, {
						transferredBytes: Math.min(job.totalBytes, Math.max(0, transferredBytes)),
					}),
				TRANSFER_CHUNK_CONCURRENCY,
			);
			updateJob(job.id, { status: 'completed', transferredBytes: job.totalBytes });
			runners.delete(job.id);
			uploadSources.get(job.id)?.dispose?.();
			uploadSources.delete(job.id);
			if (job.kind === 'upload') uploadsChanged = true;
		} catch (error) {
			if (controller.signal.aborted || (error instanceof Error && error.name === 'AbortError')) {
				updateJob(job.id, { status: 'canceled' });
			} else {
				updateJob(job.id, {
					status: 'failed',
					error: error instanceof Error ? error.message : 'Transfer failed.',
				});
			}
		} finally {
			controllers.delete(job.id);
			notifyUploadsChangedIfIdle();
		}
	}

	async function runQueue(): Promise<void> {
		if (processing || typeof window === 'undefined') return;
		processing = true;
		try {
			while (true) {
				const jobs = get(state).jobs;
				const active = jobs.filter((job) => job.status === 'running');
				const activeUploads = active.filter((job) => job.kind === 'upload');
				const maxConcurrentUploads = MAX_CONCURRENT_UPLOAD_FILES;
				const next = jobs.find((job) => {
					if (job.status !== 'queued') return false;
					if (job.kind === 'download') return active.length === 0;
					if (!active.every((activeJob) => activeJob.kind === 'upload')) return false;
					if (activeUploads.length >= maxConcurrentUploads) return false;

					return (
						job.totalBytes <= SMALL_UPLOAD_MAX_BYTES ||
						activeUploads.filter((activeJob) => activeJob.totalBytes > SMALL_UPLOAD_MAX_BYTES)
							.length < Math.min(MAX_CONCURRENT_LARGE_UPLOAD_FILES, maxConcurrentUploads)
					);
				});

				if (!next) return;

				const controller = new AbortController();
				controllers.set(next.id, controller);
				const task = runJob(next, controller).finally(() => {
					runningTasks.delete(next.id);
					void runQueue();
				});
				runningTasks.set(next.id, task);
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
			jobs: [
				...current.jobs,
				{ id, kind: 'download', fileName, totalBytes, transferredBytes: 0, status: 'queued' },
			],
			open: true,
		}));
		void runQueue();

		return id;
	}

	function cancelMany(ids: string[]): void {
		const selected = new Set(ids);

		if (!selected.size) return;
		const queued = new Set<string>();

		for (const job of get(state).jobs) {
			if (!selected.has(job.id)) continue;
			if (job.status === 'queued') queued.add(job.id);
			else if (job.status === 'running') controllers.get(job.id)?.abort();
		}
		if (queued.size) {
			state.update((current) => ({
				...current,
				jobs: current.jobs.map((job) =>
					queued.has(job.id) ? { ...job, status: 'canceled' } : job,
				),
			}));
			notifyUploadsChangedIfIdle();
		}
	}

	async function cancelAll(): Promise<void> {
		const jobs = get(state).jobs;
		cancelMany(
			jobs
				.filter((job) => job.status === 'queued' || job.status === 'running')
				.map((job) => job.id),
		);
		await Promise.all(
			jobs
				.filter((job) => job.status === 'running')
				.map((job) => runningTasks.get(job.id))
				.filter((task): task is Promise<void> => !!task),
		);
	}

	async function cancelUploads(): Promise<void> {
		const jobs = get(state).jobs;
		cancelMany(
			jobs
				.filter(
					(job) => job.kind === 'upload' && (job.status === 'queued' || job.status === 'running'),
				)
				.map((job) => job.id),
		);
		await Promise.all(
			jobs
				.filter((job) => job.kind === 'upload')
				.map((job) => runningTasks.get(job.id))
				.filter((task): task is Promise<void> => !!task),
		);
		for (const job of get(state).jobs) {
			if (job.kind === 'upload') {
				runners.delete(job.id);
				uploadSources.get(job.id)?.dispose?.();
				uploadSources.delete(job.id);
			}
		}
		state.update((current) => ({
			...current,
			jobs: current.jobs.filter((job) => job.kind !== 'upload'),
		}));
		notifyUploadsChangedIfIdle();
	}

	function retryMany(ids: string[]): void {
		const selected = new Set(ids);
		const retryable = new Set(
			get(state)
				.jobs.filter(
					(job) =>
						selected.has(job.id) &&
						runners.has(job.id) &&
						(job.status === 'failed' || job.status === 'canceled'),
				)
				.map((job) => job.id),
		);

		if (!retryable.size) return;
		state.update((current) => ({
			...current,
			jobs: current.jobs.map((job) =>
				retryable.has(job.id)
					? { ...job, status: 'queued', transferredBytes: 0, error: undefined }
					: job,
			),
		}));
		void runQueue();
	}

	function removeMany(ids: string[]): void {
		const selected = new Set(ids);
		const removed = new Set<string>();
		state.update((current) => ({
			...current,
			jobs: current.jobs.filter((job) => {
				const removable =
					selected.has(job.id) && job.status !== 'queued' && job.status !== 'running';

				if (removable) removed.add(job.id);

				return !removable;
			}),
		}));
		for (const id of removed) {
			runners.delete(id);
			uploadSources.get(id)?.dispose?.();
			uploadSources.delete(id);
		}
	}

	return {
		state,
		enqueueUploads(files, vaultId, baseFolderId) {
			if (typeof window === 'undefined' || files.length === 0) return 0;
			const folderIds = new Map<string, Promise<string>>();
			async function getFolderId(file: UploadItem, signal: AbortSignal): Promise<string | null> {
				let parentFolderId = baseFolderId;
				let path = '';

				for (const name of file.relativePath.split('/').filter(Boolean).slice(0, -1)) {
					path = path ? `${path}/${name}` : name;
					let folderId = folderIds.get(path);

					if (!folderId) {
						const currentParentId = parentFolderId;
						folderId = import('#lib/uploads/upload-file.ts')
							.then(({ createUploadFolder }) =>
								createUploadFolder(name, currentParentId, vaultId, signal),
							)
							.catch((error: unknown) => {
								folderIds.delete(path);

								throw error;
							});
						folderIds.set(path, folderId);
					}
					parentFolderId = await folderId;
				}

				return parentFolderId;
			}

			const jobs = files.map((file): Transfer => {
				const id = crypto.randomUUID();
				uploadSources.set(id, file.file);
				runners.set(id, async (signal, onProgress, concurrency) => {
					const { uploadFile } = await import('#lib/uploads/upload-file.ts');
					await uploadFile(
						file.file,
						signal,
						onProgress,
						await getFolderId(file, signal),
						concurrency,
					);
				});

				return {
					id,
					kind: 'upload',
					fileName: file.relativePath || file.file.name,
					totalBytes: file.file.size,
					transferredBytes: 0,
					status: 'queued',
				};
			});
			state.update((current) => ({ ...current, jobs: [...current.jobs, ...jobs], open: true }));
			void runQueue();

			return jobs.length;
		},
		enqueueDownload,
		setOpen(open) {
			state.update((current) => ({ ...current, open }));
		},
		cancelMany,
		cancelAll,
		cancelUploads,
		retryMany,
		removeMany,
		clearCompleted() {
			state.update((current) => ({
				...current,
				jobs: current.jobs.filter((job) => job.status !== 'completed'),
			}));
		},
	};
}
