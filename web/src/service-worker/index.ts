import { self } from '$app/service-worker';

interface StreamSession {
	sessionId: string;
	purpose: 'preview' | 'download';
	fileId: string;
	filename: string;
	plaintextSize: number;
	mimeType: string;
	concurrency: number;
	clientId: string;
	downloadOffset: number;
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

interface ByteRange {
	start: number;
	end: number;
}

interface PendingRead {
	resolve(bytes: Uint8Array<ArrayBuffer>): void;
	reject(error: Error): void;
}

const sessions = new Map<string, StreamSession>();
const pendingReads = new Map<string, PendingRead>();
const INITIAL_PROBE_BYTES = 4 * 1024 * 1024;
const MAX_STREAM_RESPONSE_BYTES = 4 * 1024 * 1024;
const STREAM_READ_TIMEOUT_MS = 5 * 60 * 1000;
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
      filename: String(session.filename || "download"),
      plaintextSize: Number(session.plaintextSize || 0),
      mimeType: String(session.mimeType || "application/octet-stream"),
      concurrency: Math.min(8, Math.max(1, Math.floor(Number(session.concurrency || 1)))),
      clientId,
      downloadOffset: 0,
      maxLifetimeMs,
      expiresAt: now + maxLifetimeMs,
    });
    return;
  }

  if (message.type === "ARKIVE_STREAM_CLOSE") {
    sessions.delete(String(message.sessionId || ""));
    return;
  }

  if (message.type === "ARKIVE_STREAM_READ_RESULT") {
    const key = String(message.sessionId || "") + ":" + String(message.requestId || "");
    const pending = pendingReads.get(key);
    if (!pending) {
      return;
    }
    pendingReads.delete(key);
    pending.resolve(new Uint8Array(message.bytes || new ArrayBuffer(0)));
    return;
  }

  if (message.type === "ARKIVE_STREAM_READ_ERROR") {
    const key = String(message.sessionId || "") + ":" + String(message.requestId || "");
    const pending = pendingReads.get(key);
    if (!pending) {
      return;
    }
    pendingReads.delete(key);
    pending.reject(new Error(String(message.error || "Stream read failed.")));
  }
});

self.addEventListener("fetch", function(event) {
  cleanupExpiredSessions();
  const url = new URL(event.request.url);
  if (!url.pathname.startsWith("/arkive-stream/") && !url.pathname.startsWith("/arkive-download/")) {
    return;
  }
  event.respondWith(
    handleRequest(event.request, url, event.clientId).catch(function(error: unknown) {
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

function parseRange(header: string | null, size: number): ByteRange {
  if (!header || !header.startsWith("bytes=")) {
    return {
      start: 0,
      end: Math.min(Math.max(0, size - 1), INITIAL_PROBE_BYTES - 1),
    };
  }

  const parts = header.replace("bytes=", "").split("-");
  const startRaw = parts[0];
  const endRaw = parts[1];

  if (startRaw === "" && endRaw) {
    const suffixSize = Math.max(0, Number(endRaw || 0));
    return {
      start: Math.max(0, size - suffixSize),
      end: Math.max(0, size - 1),
    };
  }

  const start = Number(startRaw || 0);
  const end = endRaw
    ? Number(endRaw)
    : Math.min(size - 1, start + MAX_STREAM_RESPONSE_BYTES - 1);

  return {
    start: Math.max(0, start),
    end: Math.min(size - 1, end),
  };
}

function clampRange(range: ByteRange, size: number): ByteRange {
  const start = Math.max(0, Number(range && range.start) || 0);
  const requestedEnd = Math.min(size - 1, Math.max(start, Number(range && range.end) || start));
  const end = Math.min(requestedEnd, start + MAX_STREAM_RESPONSE_BYTES - 1);

  return {
    start: start,
    end: end,
  };
}

async function handleRequest(request: Request, url: URL, clientId: string): Promise<Response> {
  if (isDownloadRequest(url)) {
    return handleDownloadRequest(url, clientId);
  }
  return handleStreamRequest(request, url, clientId);
}

async function handleStreamRequest(request: Request, url: URL, clientId: string): Promise<Response> {
  const session = findSession(url, clientId, 'preview');
  if (!session) {
    return new Response("Stream session not found.", { status: 404 });
  }
  if (isSessionExpired(session)) {
    finalizeSession(session, "expired");
    return new Response("Stream session expired.", { status: 410 });
  }
  touchSession(session);

  const range = clampRange(
    parseRange(request.headers.get("Range"), session.plaintextSize),
    session.plaintextSize,
  );
  if (range.end < range.start || session.plaintextSize <= 0) {
    return new Response(null, {
      status: 416,
      headers: {
        "Content-Range": "bytes */" + String(session.plaintextSize || 0),
      },
    });
  }

  const bytes = await requestBytesFromClient(session, range.start, range.end);

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

async function handleDownloadRequest(url: URL, clientId: string): Promise<Response> {
  const session = findSession(url, clientId, 'download');
  if (!session) {
    return new Response("Download session not found.", { status: 404 });
  }
  if (isSessionExpired(session)) {
    finalizeSession(session, "expired");
    return new Response("Download session expired.", { status: 410 });
  }
  touchSession(session);

  const headers = {
    "Cache-Control": "no-store",
    "Content-Type": session.mimeType || "application/octet-stream",
    "Content-Disposition": buildContentDisposition(session.filename || "download"),
    "Content-Length": String(session.plaintextSize || 0),
  };

  const stream = new ReadableStream<Uint8Array>({
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
          requestBytesFromClient(session, range.start, range.end)
        ));
        const failed = results.find((result) => result.status === 'rejected');
        if (failed?.status === 'rejected') throw failed.reason;

        for (let i = 0; i < ranges.length; i++) {
          touchSession(session);
          session.downloadOffset = ranges[i].end + 1;
          const result = results[i];
          if (result?.status === 'fulfilled') controller.enqueue(result.value);
        }
        if (session.downloadOffset >= total) {
          controller.close();
          finalizeSession(session, "complete");
        }
      } catch (error) {
        controller.error(error);
        finalizeSession(session, "error");
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
	// The URL acts as a temporary capability, so tie it to its file and original page.
	if (!clientId || !session || session.clientId !== clientId || session.fileId !== fileId || session.purpose !== purpose) return;
	return session;
}

async function requestBytesFromClient(session: StreamSession, start: number, end: number): Promise<Uint8Array<ArrayBuffer>> {
  if (isSessionExpired(session)) {
    throw new Error("Stream session expired.");
  }
  const client = await self.clients.get(session.clientId);
  if (!client) {
    throw new Error("Stream page is unavailable.");
  }

  const requestId = String(Date.now()) + "-" + Math.random().toString(36).slice(2);
  const key = session.sessionId + ":" + requestId;

  return new Promise<Uint8Array<ArrayBuffer>>(function(resolve, reject) {
    pendingReads.set(key, { resolve: resolve, reject: reject });

    client.postMessage({
      type: "ARKIVE_STREAM_READ",
      sessionId: session.sessionId,
      requestId: requestId,
      start: start,
      end: end,
    });

    setTimeout(function() {
      const pending = pendingReads.get(key);
      if (!pending) {
        return;
      }
      pendingReads.delete(key);
      reject(new Error("Timed out waiting for encrypted stream data."));
    }, STREAM_READ_TIMEOUT_MS);
  });
}

function buildContentDisposition(filename: string): string {
  const fallback = String(filename || "download").replace(/[^\x20-\x7E]+/g, "_").replace(/["\\]/g, "_");
  const encoded = encodeURIComponent(String(filename || "download")).replace(/['()]/g, escape).replace(/\*/g, "%2A");
  return 'attachment; filename="' + fallback + '"; filename*=UTF-8\'\'' + encoded;
}

function finalizeSession(session: StreamSession | undefined, status: string): void {
  if (!session || !session.sessionId) {
    return;
  }
  sessions.delete(String(session.sessionId));
  notifyClient(session.clientId, {
    type: "ARKIVE_STREAM_FINISH",
    sessionId: String(session.sessionId),
    status: String(status || "complete"),
  });
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

async function notifyClient(clientId: string, message: StreamMessage & { type: string; sessionId: string; status: string }): Promise<void> {
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
