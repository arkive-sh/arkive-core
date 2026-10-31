interface StreamRecord {
	fileId: string;
	plaintextSize: number;
}

interface StreamMetadata {
	name?: string;
	mime?: string;
	size?: number;
}

interface StreamRegistration {
	sessionId: string;
	purpose: 'preview' | 'download';
	fileId: string;
	filename: string;
	plaintextSize: number;
	mimeType: string;
	concurrency: number;
}

interface StreamOptions {
	reader: StreamReader;
	record: StreamRecord;
	metadata: StreamMetadata;
	purpose?: StreamRegistration['purpose'];
	concurrency?: number;
}

export interface StreamReader {
	readRange(start: number, end: number, signal?: AbortSignal): Promise<Uint8Array>;
}

export interface EncryptedStream {
	url: string;
	close(): void;
}

const providers = new Map<string, { reader: StreamReader; reads: Map<string, AbortController> }>();
let providerBridgeBound = false;

function cancelProviderReads(provider: { reads: Map<string, AbortController> }): void {
	for (const controller of provider.reads.values()) controller.abort();
	provider.reads.clear();
}

function postToServiceWorker(message: Record<string, unknown>, transfer: Transferable[] = []): void {
	const controller = navigator.serviceWorker.controller;
	if (!controller) throw new Error('Streaming service worker is not controlling this page yet.');
	controller.postMessage(message, transfer);
}

function bindProviderBridge(): void {
	if (providerBridgeBound) return;
	providerBridgeBound = true;
	navigator.serviceWorker.addEventListener('message', (event: MessageEvent<Record<string, unknown>>) => {
		const message = event.data;
		const sessionId = String(message.sessionId || '');
		const provider = providers.get(sessionId);
		if (message.type === 'ARKIVE_STREAM_FINISH') {
			if (provider) cancelProviderReads(provider);
			providers.delete(sessionId);
			return;
		}
		if (message.type === 'ARKIVE_STREAM_READ_CANCEL') {
			const requestId = String(message.requestId || '');
			provider?.reads.get(requestId)?.abort();
			provider?.reads.delete(requestId);
			return;
		}
		if (message.type !== 'ARKIVE_STREAM_READ') return;

		const requestId = String(message.requestId || '');
		if (!provider) {
			try {
				postToServiceWorker({ type: 'ARKIVE_STREAM_READ_ERROR', sessionId, requestId, error: 'Stream session is unavailable.' });
			} catch {}
			return;
		}

		// Cancel the storage fetch and discard any decrypt result when this range is abandoned.
		const controller = new AbortController();
		provider.reads.set(requestId, controller);
		void provider.reader.readRange(Number(message.start), Number(message.end) + 1, controller.signal)
			.then((bytes) => {
				if (controller.signal.aborted) {
					bytes.fill(0);
					return;
				}
				const buffer = bytes.buffer as ArrayBuffer;
				postToServiceWorker({ type: 'ARKIVE_STREAM_READ_RESULT', sessionId, requestId, bytes: buffer }, [buffer]);
			})
			.catch((error: unknown) => {
				if (controller.signal.aborted) return;
				try {
					postToServiceWorker({
						type: 'ARKIVE_STREAM_READ_ERROR',
						sessionId,
						requestId,
						error: error instanceof Error ? error.message : 'Stream read failed.'
					});
				} catch {}
			})
			.finally(() => provider.reads.delete(requestId));
	});
}

async function waitForServiceWorkerController(): Promise<ServiceWorker> {
	if (!('serviceWorker' in navigator) || !window.isSecureContext) {
		throw new Error('Encrypted streaming requires a secure connection and service workers.');
	}
	if (navigator.serviceWorker.controller) return navigator.serviceWorker.controller;

	await navigator.serviceWorker.register('/service-worker.js', { scope: '/', type: 'module' });
	return new Promise((resolve, reject) => {
		const timer = window.setTimeout(() => finish(new Error('Streaming service worker is not ready. Reload the page and try again.')), 8000);
		const onControllerChange = () => {
			if (navigator.serviceWorker.controller) finish();
		};
		const finish = (error?: Error) => {
			window.clearTimeout(timer);
			navigator.serviceWorker.removeEventListener('controllerchange', onControllerChange);
			if (error) reject(error);
			else if (navigator.serviceWorker.controller) resolve(navigator.serviceWorker.controller);
			else reject(new Error('Streaming service worker is not controlling this page.'));
		};
		navigator.serviceWorker.addEventListener('controllerchange', onControllerChange);
		onControllerChange();
	});
}

export async function openEncryptedStream(options: StreamOptions): Promise<EncryptedStream> {
	bindProviderBridge();
	await waitForServiceWorkerController();

	const registration: StreamRegistration = {
		sessionId: crypto.randomUUID(),
		purpose: options.purpose || 'preview',
		fileId: options.record.fileId,
		filename: options.metadata.name || 'download',
		plaintextSize: options.metadata.size ?? options.record.plaintextSize,
		mimeType: options.metadata.mime || 'application/octet-stream',
		concurrency: Math.min(8, Math.max(1, Math.floor(options.concurrency || 1)))
	};

	providers.set(registration.sessionId, { reader: options.reader, reads: new Map() });
	try {
		postToServiceWorker({ type: 'ARKIVE_STREAM_OPEN', session: registration });
	} catch (error) {
		providers.delete(registration.sessionId);
		throw error;
	}

	const endpoint = registration.purpose === 'download' ? 'arkive-download' : 'arkive-stream';
	let closed = false;
	return {
		url: `/${endpoint}/${encodeURIComponent(registration.fileId)}?session=${encodeURIComponent(registration.sessionId)}`,
		close() {
			if (closed) return;
			closed = true;
			const provider = providers.get(registration.sessionId);
			if (provider) cancelProviderReads(provider);
			providers.delete(registration.sessionId);
			try {
				postToServiceWorker({ type: 'ARKIVE_STREAM_CLOSE', sessionId: registration.sessionId });
			} catch {}
		}
	};
}

export async function startServiceWorkerDownload(options: Omit<StreamOptions, 'purpose'>): Promise<EncryptedStream> {
	const stream = await openEncryptedStream({ ...options, purpose: 'download' });
	const anchor = document.createElement('a');
	anchor.href = stream.url;
	anchor.download = options.metadata.name || 'download';
	anchor.rel = 'noopener';
	anchor.hidden = true;
	document.body.appendChild(anchor);
	anchor.click();
	anchor.remove();
	return stream;
}
