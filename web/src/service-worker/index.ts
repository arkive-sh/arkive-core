import { self } from '$app/service-worker';
import { parseByteRange } from './range.js';

interface StreamSession {
	sessionId: string;
	purpose: 'preview' | 'download';
	fileId: string;
	plaintextSize: number;
	mimeType: string;
	concurrency: number;
	clientId: string;
	downloadOffset: number;
	downloadController?: ReadableStreamDefaultController<Uint8Array>;
	maxLifetimeMs: number;
	expiresAt: number;
}

interface StreamMessage {
	type?: string;
	session?: Partial<StreamSession>;
	sessionId?: string;
	requestId?: string;
	bytes?: ArrayBuffer;
	error?: string;
}

interface PendingRead {
	sessionId: string;
	requestId: string;
	clientId: string;
	resolve(bytes: Uint8Array<ArrayBuffer>): void;
	reject(error: Error): void;
	timeout?: ReturnType<typeof setTimeout>;
	signal?: AbortSignal;
	onAbort?: () => void;
}

interface PendingRestore {
	promise: Promise<void>;
	resolve(): void;
	timeout: ReturnType<typeof setTimeout>;
}

const sessions = new Map<string, StreamSession>();
const pendingReads = new Map<string, PendingRead>();
const pendingRestores = new Map<string, PendingRestore>();
const MAX_STREAM_RESPONSE_BYTES = 4 * 1024 * 1024;
const STREAM_READ_TIMEOUT_MS = 5 * 60 * 1000;
const STREAM_RESTORE_TIMEOUT_MS = 1000;
const PREVIEW_SESSION_MAX_LIFETIME_MS = 6 * 60 * 60 * 1000;
const DOWNLOAD_SESSION_MAX_LIFETIME_MS = 45 * 60 * 1000;

self.addEventListener("install", function(event) {
  event.waitUntil(self.skipWaiting());
});

self.addEventListener("activate", function(event) {
  event.waitUntil(self.clients.claim());
});

self.addEventListener("message", function(event) {
  cleanupExpiredSessions();
  const message = (event.data || {}) as StreamMessage;

  if (message.type === "ARKIVE_STREAM_OPEN" && message.session) {
    const now = Date.now();
    const session = message.session;
    // Use the browser-provided opener ID so another tab cannot claim this stream.
    const clientId = event.source && 'id' in event.source ? String(event.source.id) : "";
    if (!clientId || !session.sessionId || !session.fileId || (session.purpose !== 'preview' && session.purpose !== 'download')) return;
    const maxLifetimeMs = sessionLifetimeMs(session);
    sessions.set(session.sessionId, {
      sessionId: session.sessionId,
      purpose: session.purpose,
      fileId: session.fileId,
      plaintextSize: Number(session.plaintextSize || 0),
      mimeType: String(session.mimeType || "application/octet-stream"),
      concurrency: Math.min(8, Math.max(1, Math.floor(Number(session.concurrency || 1)))),
      clientId,
      downloadOffset: 0,
      maxLifetimeMs,
      expiresAt: now + maxLifetimeMs,
    });
    finishSessionRestore(session.sessionId);
    return;
  }

  if (message.type === "ARKIVE_STREAM_CLOSE") {
    const sessionId = String(message.sessionId || "");
    finishSessionRestore(sessionId);
    const session = sessions.get(sessionId);
    if (session) finalizeSession(session, 'closed');
    else cancelSessionReads(sessionId, 'closed');
    return;
  }

  if (message.type === "ARKIVE_STREAM_READ_RESULT") {
    const key = String(message.sessionId || "") + ":" + String(message.requestId || "");
    const pending = takePendingRead(key);
    if (!pending) {
      if (message.bytes) new Uint8Array(message.bytes).fill(0);
      return;
    }
    pending.resolve(new Uint8Array(message.bytes || new ArrayBuffer(0)));
    return;
  }

  if (message.type === "ARKIVE_STREAM_READ_ERROR") {
    const key = String(message.sessionId || "") + ":" + String(message.requestId || "");
    const pending = takePendingRead(key);
    if (!pending) {
      return;
    }
    pending.reject(new Error(String(message.error || "Stream read failed.")));
  }
});

self.addEventListener("fetch", function(event) {
  cleanupExpiredSessions();
  const url = new URL(event.request.url);
  if (!url.pathname.startsWith("/arkive-stream/") && !url.pathname.startsWith("/arkive-download/")) {
    return;
  }
	const clientId = event.clientId;
  event.respondWith(
    handleRequest(event.request, url, clientId).catch(async function(error: unknown) {
      if (isDownloadRequest(url)) {
        const sessionId = url.searchParams.get('session');
        if (sessionId) await notifyClient(clientId, {
          type: 'ARKIVE_STREAM_FINISH',
          sessionId,
          status: 'error',
          error: error instanceof Error ? error.message : 'Encrypted download failed.'
        });
      }
      return new Response(error instanceof Error ? error.message : "Encrypted stream failed.", {
        status: 504,
        headers: {
          "Cache-Control": "no-store",
          "Content-Type": "text/plain; charset=utf-8",
        },
      });
    }),
  );
});

function isDownloadRequest(url: URL): boolean {
  return url.pathname.startsWith("/arkive-download/");
}

async function handleRequest(request: Request, url: URL, clientId: string): Promise<Response> {
  if (isDownloadRequest(url)) {
    return handleDownloadRequest(url, clientId, request.signal);
  }
  return handleStreamRequest(request, url, clientId);
}

async function handleStreamRequest(request: Request, url: URL, clientId: string): Promise<Response> {
  const session = await findOrRestoreSession(url, clientId, 'preview');
  if (!session) {
    return new Response("Stream session not found.", { status: 404 });
  }
  if (isSessionExpired(session)) {
    finalizeSession(session, "expired");
    return new Response("Stream session expired.", { status: 410 });
  }
  touchSession(session);

  const range = parseByteRange(request.headers.get('Range'), session.plaintextSize, MAX_STREAM_RESPONSE_BYTES);
  if (!range) {
    return new Response(null, {
      status: 416,
      headers: {
        "Content-Range": "bytes */" + String(session.plaintextSize || 0),
      },
    });
  }

  const bytes = await requestBytesFromClient(session, range.start, range.end, request.signal);
  if (request.signal.aborted) {
    bytes.fill(0);
    request.signal.throwIfAborted();
  }

  return new Response(bytes.buffer, {
    status: 206,
    headers: {
      "Accept-Ranges": "bytes",
      "Cache-Control": "no-store",
      "Content-Length": String(bytes.byteLength || bytes.length || 0),
      "Content-Range": "bytes " + String(range.start) + "-" + String(range.end) + "/" + String(session.plaintextSize),
      "Content-Type": session.mimeType || "application/octet-stream",
    },
  });
}

async function handleDownloadRequest(url: URL, clientId: string, signal: AbortSignal): Promise<Response> {
  const session = await findOrRestoreSession(url, clientId, 'download');
  if (!session) {
    const sessionId = url.searchParams.get('session');
    if (sessionId) await notifyClient(clientId, {
      type: 'ARKIVE_STREAM_FINISH',
      sessionId,
      status: 'error',
      error: 'Download session could not be found.'
    });
    return new Response("Download session not found.", { status: 404 });
  }
  if (isSessionExpired(session)) {
    finalizeSession(session, "expired", "Download session expired.");
    return new Response("Download session expired.", { status: 410 });
  }
  touchSession(session);

  const headers = {
    "Cache-Control": "no-store",
    "Content-Type": session.mimeType || "application/octet-stream",
    "Content-Length": String(session.plaintextSize || 0),
  };

  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      session.downloadController = controller;
    },
    async pull(controller) {
      const total = Number(session.plaintextSize || 0);
      if (Number(session.downloadOffset || 0) >= total) {
        controller.close();
        finalizeSession(session, "complete");
        return;
      }

      try {
        const ranges: ByteRange[] = [];
        let offset = Number(session.downloadOffset || 0);
        for (let i = 0; i < session.concurrency && offset < total; i++) {
          const end = Math.min(total - 1, offset + MAX_STREAM_RESPONSE_BYTES - 1);
          ranges.push({ start: offset, end: end });
          offset = end + 1;
        }
        const results = await Promise.allSettled(ranges.map((range) =>
          requestBytesFromClient(session, range.start, range.end, signal)
        ));
        const failed = results.find((result) => result.status === 'rejected');
        if (signal.aborted || failed?.status === 'rejected') {
          for (const result of results) {
            if (result.status === 'fulfilled') result.value.fill(0);
          }
          signal.throwIfAborted();
          if (failed?.status === 'rejected') throw failed.reason;
        }

        for (let i = 0; i < ranges.length; i++) {
          const result = results[i];
          if (result?.status === 'fulfilled') {
            controller.enqueue(result.value);
            session.downloadOffset = ranges[i].end + 1;
            touchSession(session);
          }
        }
        if (session.downloadOffset >= total) {
          controller.close();
          finalizeSession(session, "complete");
        }
      } catch (error) {
        if (!sessions.has(session.sessionId)) return;
        controller.error(error);
        finalizeSession(session, "error", error instanceof Error ? error.message : 'Encrypted download failed.');
      }
    },
    cancel() {
      finalizeSession(session, "cancelled");
    },
  });

  return new Response(stream, {
    status: 200,
    headers: headers,
  });
}

function findSession(url: URL, clientId: string, purpose: StreamSession['purpose']): StreamSession | undefined {
	const prefix = purpose === 'download' ? '/arkive-download/' : '/arkive-stream/';
	const encodedFileId = url.pathname.slice(prefix.length);
	if (!url.pathname.startsWith(prefix) || !encodedFileId || encodedFileId.includes('/')) return;

	let fileId: string;
	try {
		fileId = decodeURIComponent(encodedFileId);
	} catch {
		return;
	}

	const sessionId = url.searchParams.get('session');
	const session = sessionId ? sessions.get(sessionId) : undefined;
	// Stream URLs remain bound to the page that registered them.
	if (!clientId || !session || session.clientId !== clientId || session.fileId !== fileId || session.purpose !== purpose) return;
	return session;
}

async function findOrRestoreSession(url: URL, clientId: string, purpose: StreamSession['purpose']): Promise<StreamSession | undefined> {
	let session = findSession(url, clientId, purpose);
	if (session || !clientId) return session;

	const sessionId = url.searchParams.get('session');
	if (!sessionId) return;
	const client = await self.clients.get(clientId);
	if (!client) return;
	session = findSession(url, clientId, purpose);
	if (session) return session;

	let pending = pendingRestores.get(sessionId);
	if (!pending) {
		let resolve!: () => void;
		const promise = new Promise<void>((done) => { resolve = done; });
		pending = {
			promise,
			resolve,
			timeout: setTimeout(() => finishSessionRestore(sessionId), STREAM_RESTORE_TIMEOUT_MS)
		};
		pendingRestores.set(sessionId, pending);
		// The worker lost its map on restart; ask the page to replay its live registration.
		try {
			client.postMessage({ type: 'ARKIVE_STREAM_RESTORE', sessionId });
		} catch {
			finishSessionRestore(sessionId);
		}
	}

	await pending.promise;
	return findSession(url, clientId, purpose);
}

function finishSessionRestore(sessionId: string): void {
	const pending = pendingRestores.get(sessionId);
	if (!pending) return;
	clearTimeout(pending.timeout);
	pendingRestores.delete(sessionId);
	pending.resolve();
}

async function requestBytesFromClient(session: StreamSession, start: number, end: number, signal?: AbortSignal): Promise<Uint8Array<ArrayBuffer>> {
  if (isSessionExpired(session)) {
    throw new Error("Stream session expired.");
  }
  signal?.throwIfAborted();
  const client = await self.clients.get(session.clientId);
  if (!client) {
    throw new Error("Stream page is unavailable.");
  }

  const requestId = String(Date.now()) + "-" + Math.random().toString(36).slice(2);
  const key = session.sessionId + ":" + requestId;

  return new Promise<Uint8Array<ArrayBuffer>>((resolve, reject) => {
    const pending: PendingRead = {
      sessionId: session.sessionId,
      requestId,
      clientId: session.clientId,
      resolve,
      reject,
      signal,
    };
    pendingReads.set(key, pending);
    pending.timeout = setTimeout(() => cancelPendingRead(key, new Error('Timed out waiting for encrypted stream data.')), STREAM_READ_TIMEOUT_MS);

    if (signal) {
      pending.onAbort = () => cancelPendingRead(key, new DOMException('Stream read was cancelled.', 'AbortError'));
      signal.addEventListener('abort', pending.onAbort, { once: true });
      if (signal.aborted) {
        pending.onAbort();
        return;
      }
    }

    try {
      client.postMessage({
        type: "ARKIVE_STREAM_READ",
        sessionId: session.sessionId,
        requestId: requestId,
        start: start,
        end: end,
      });
    } catch (error) {
      cancelPendingRead(key, error instanceof Error ? error : new Error('Could not request encrypted stream data.'));
    }
  });
}

function finalizeSession(session: StreamSession | undefined, status: string, error?: string): void {
  if (!session || !session.sessionId) {
    return;
  }
  if (session.purpose === 'download' && status !== 'complete') {
    try {
      session.downloadController?.error(new DOMException('Download stream closed.', 'AbortError'));
    } catch {}
  }
  cancelSessionReads(session.sessionId, status);
  sessions.delete(String(session.sessionId));
  notifyClient(session.clientId, {
    type: "ARKIVE_STREAM_FINISH",
    sessionId: String(session.sessionId),
    status: String(status || "complete"),
    error,
  });
}

function takePendingRead(key: string): PendingRead | undefined {
	// Settled requests must release their fallback timer and abort listener immediately.
	const pending = pendingReads.get(key);
	if (!pending) return;
	pendingReads.delete(key);
	if (pending.timeout !== undefined) clearTimeout(pending.timeout);
	if (pending.signal && pending.onAbort) pending.signal.removeEventListener('abort', pending.onAbort);
	return pending;
}

function cancelPendingRead(key: string, error: Error): void {
	const pending = takePendingRead(key);
	if (!pending) return;
	pending.reject(error);
	void notifyClient(pending.clientId, {
		type: 'ARKIVE_STREAM_READ_CANCEL',
		sessionId: pending.sessionId,
		requestId: pending.requestId
	});
}

function cancelSessionReads(sessionId: string, reason: string): void {
	// Closing a stream must stop the page-side range reads already dispatched for it.
	for (const [key, pending] of pendingReads) {
		if (pending.sessionId === sessionId) {
			cancelPendingRead(key, new DOMException(`Stream ${reason}.`, 'AbortError'));
		}
	}
}

function touchSession(session: StreamSession): void {
  if (!session) {
    return;
  }
  session.expiresAt = Date.now() + Number(session.maxLifetimeMs || PREVIEW_SESSION_MAX_LIFETIME_MS);
}

function isSessionExpired(session: StreamSession | undefined): boolean {
  return !session || Number(session.expiresAt || 0) <= Date.now();
}

function cleanupExpiredSessions() {
  sessions.forEach(function(session) {
    if (isSessionExpired(session)) {
      finalizeSession(session, "expired");
    }
  });
}

function sessionLifetimeMs(session: Partial<StreamSession>): number {
  if (session && String(session.purpose || "") === "download") {
    return DOWNLOAD_SESSION_MAX_LIFETIME_MS;
  }
  return PREVIEW_SESSION_MAX_LIFETIME_MS;
}

async function notifyClient(clientId: string, message: Record<string, unknown>): Promise<void> {
  if (!clientId) {
    return;
  }
  try {
    const client = await self.clients.get(clientId);
    if (client) {
      client.postMessage(message);
    }
  } catch (_) {}
}
