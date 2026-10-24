import { fetchEncryptedChunk } from "./chunk_fetcher.js";

export async function downloadBlobFallback(options) {
  const record = options.record;
  const chunkMap = options.chunkMap || [];
  const decryptChunk = options.decryptChunk;
  const filename = options.filename;
  const signal = options.signal;
  const onProgress = options.onProgress;
  const concurrency = Math.min(8, Math.max(1, Number(options.concurrency || 1)));
  const parts = new Array(chunkMap.length);
  let written = 0;
  let nextIndex = 0;
  let failure = null;

  async function worker() {
    while (nextIndex < chunkMap.length && !failure) {
      const i = nextIndex++;
      const chunk = chunkMap[i];
      try {
        if (signal && signal.aborted) {
          throw new DOMException("Download aborted", "AbortError");
        }

        const encryptedBuffer = await fetchEncryptedChunk(record.sourceUrl, chunk, signal);
        const plainBuffer = await decryptChunk(chunk, encryptedBuffer);
        parts[i] = plainBuffer;

        written += Number(chunk.plainSize || plainBuffer.length || 0);
        if (onProgress) {
          onProgress({
            written: written,
            total: Number(record.plaintextSize || 0),
            chunkIndex: chunk.index,
          });
        }
      } catch (error) {
        failure = error;
      }
    }
  }

  await Promise.all(Array.from({ length: Math.min(concurrency, chunkMap.length) }, worker));
  if (failure) throw failure;

  const blob = new Blob(parts, {
    type: record.mimeType || "application/octet-stream",
  });
  const url = URL.createObjectURL(blob);

  try {
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = filename || record.filename || "download";
    anchor.rel = "noopener";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  } finally {
    window.setTimeout(function() {
      URL.revokeObjectURL(url);
    }, 30000);
  }
}
