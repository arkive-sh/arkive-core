// Shared download transfer manager. Download state (active transfer, cancel,
// status panel) lives here instead of page modules, so a download keeps
// running across soft navigation. The status panel nodes are rendered in the
// auth shell (layout_auth.go), outside .app-content, so their ids survive
// every content swap.
import { ArkiveFileReader } from "./file_reader.js";
import { isDownloadAbortError, showDownloadError, showServiceWorkerDownloadNotice } from "./reader/download_warning.js";
import { showAppError } from "../lib/toasts.js";

const HIDE_QUEUE_DELAY_MS = 1200;

let activeController = null;
let cancelledByUser = false;
let hideQueueTimer = 0;

function node(id) {
  return document.getElementById(id);
}

// auth_layout.css keys the shell download panel off body[data-downloading],
// so the panel follows an active transfer onto any page it is carried to.
function markDownloading(active) {
  if (!document.body) {
    return;
  }
  if (active) {
    document.body.setAttribute("data-downloading", "true");
  } else {
    document.body.removeAttribute("data-downloading");
  }
}

function formatBytes(bytes) {
  const value = Number(bytes || 0);
  if (value <= 0) {
    return "0 B";
  }
  const units = ["B", "KB", "MB", "GB", "TB"];
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1);
  const sized = value / Math.pow(1024, index);
  return sized.toFixed(index === 0 ? 0 : 1) + " " + units[index];
}

function clearHideQueueTimer() {
  if (!hideQueueTimer) {
    return;
  }
  window.clearTimeout(hideQueueTimer);
  hideQueueTimer = 0;
}

function setDownloadState(state, detail) {
  const item = node("download-queue-item");
  if (item) {
    item.className = "queue-item" + (state === "complete" ? " is-complete" : state === "error" || state === "cancelled" ? " is-error" : "");
  }
  const badge = node("download-status-badge");
  if (badge) {
    badge.className = "queue-item-badge is-" + state;
    badge.textContent = state;
  }
  const statusDetail = node("download-status-detail");
  if (statusDetail) {
    statusDetail.textContent = detail || state;
  }
  const queueMeta = node("download-queue-meta");
  if (queueMeta) {
    queueMeta.textContent = state === "running" ? "1 item active" : "0 items active";
  }
  const cancelButton = node("download-cancel");
  if (cancelButton) {
    cancelButton.hidden = state !== "running";
  }
}

function showDownloadQueue(fileName) {
  clearHideQueueTimer();
  const queue = node("download-queue");
  if (queue) {
    queue.hidden = false;
  }
  const label = node("download-file-label");
  if (label) {
    label.textContent = fileName || "Encrypted file";
  }
}

function hideDownloadQueueSoon() {
  clearHideQueueTimer();
  hideQueueTimer = window.setTimeout(function() {
    const queue = node("download-queue");
    if (queue) {
      queue.hidden = true;
    }
  }, HIDE_QUEUE_DELAY_MS);
}

function resetProgress() {
  clearHideQueueTimer();
  const fill = node("download-progress-fill");
  if (fill) {
    fill.style.width = "0%";
  }
  const progressText = node("download-progress-text");
  if (progressText) {
    progressText.textContent = "0 B / 0 B";
  }
  setDownloadState("queued", "queued");
}

function updateProgress(written, total) {
  const pct = total > 0 ? Math.round((written / total) * 100) : 0;
  const fill = node("download-progress-fill");
  if (fill) {
    fill.style.width = pct + "%";
  }
  const progressText = node("download-progress-text");
  if (progressText) {
    progressText.textContent = pct + "% - " + formatBytes(written) + " of " + formatBytes(total);
  }
  setDownloadState("running", "Downloading");
}

export function cancelActiveDownload() {
  if (!activeController) {
    return;
  }
  const warning = node("download-warning");
  if (warning) {
    warning.innerHTML = "";
  }
  cancelledByUser = true;
  activeController.abort();
  activeController = null;
  // downloadFile's finally guard compares against activeController, which is
  // already null here, so clear the marker here or it leaks on <body>.
  markDownloading(false);
  setDownloadState("cancelled", "Cancelled");
  hideDownloadQueueSoon();
}

// Starts a download with its own reader, so page-scoped teardown (media
// preview, file lists) cannot kill it mid-transfer.
export async function downloadFile(fileId, options) {
  const settings = options || {};
  if (!fileId || activeController) {
    return;
  }
  const trigger = settings.trigger || null;
  const warningContainer = settings.warningContainer || null;
  if (trigger) {
    trigger.disabled = true;
  }
  const warning = node("download-warning");
  if (warning) {
    warning.innerHTML = "";
  }
  const reader = new ArkiveFileReader({ fileId: fileId });
  const controller = new AbortController();
  activeController = controller;
  cancelledByUser = false;
  markDownloading(true);
  resetProgress();
  showDownloadQueue("Encrypted file");
  setDownloadState("running", "Preparing");
  try {
    await reader.load();
    showDownloadQueue(reader.getMetadata().name || "Encrypted file");
    const result = await reader.download({
      warningContainer: warningContainer,
      signal: controller.signal,
      onProgress: function(progress) {
        updateProgress(progress.written, progress.total);
      },
    });
    if (result && result.mode === "warning") {
      const queue = node("download-queue");
      if (queue) {
        queue.hidden = true;
      }
      return;
    }
    if (result && result.mode === "service-worker" && warningContainer) {
      showServiceWorkerDownloadNotice(warningContainer);
    }
    setDownloadState("complete", result && result.mode === "service-worker" ? "Browser download started" : "Saved");
    hideDownloadQueueSoon();
  } catch (error) {
    if (isDownloadAbortError(error)) {
      if (!cancelledByUser) {
        // Not a user cancel (page context went away): drop the queue quietly.
        const queue = node("download-queue");
        if (queue) {
          queue.hidden = true;
        }
      }
      return;
    }
    setDownloadState("error", "Download failed");
    if (warningContainer) {
      showDownloadError(warningContainer, (error && error.message) || "Download failed.");
    }
    showAppError(error, {
      code: "download_failed",
      message: "Download failed.",
    });
  } finally {
    if (activeController === controller) {
      activeController = null;
      markDownloading(false);
    }
    if (trigger) {
      trigger.disabled = false;
    }
    await reader.dispose().catch(function() {});
  }
}

const cancelButton = document.getElementById("download-cancel");
if (cancelButton) {
  cancelButton.addEventListener("click", function() {
    cancelActiveDownload();
  });
}
