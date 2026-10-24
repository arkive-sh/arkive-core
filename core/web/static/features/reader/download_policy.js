export const DOWNLOAD_POLICY = Object.freeze({
  blobFallbackLimitIOS: 128 * 1024 * 1024,
  blobFallbackLimitDesktop: 512 * 1024 * 1024,
});

let transferConcurrencyPromise;

export function getTransferConcurrency() {
  if (!transferConcurrencyPromise) {
    transferConcurrencyPromise = fetch("/api/transfers/limits", { credentials: "include" })
      .then(function(response) {
        if (!response.ok) return 3;
        return response.json();
      })
      .then(function(data) {
        const value = Number(data && data.concurrency);
        return Number.isFinite(value) ? Math.min(8, Math.max(1, Math.floor(value))) : 3;
      })
      .catch(function() {
        return 3;
      });
  }
  return transferConcurrencyPromise;
}
