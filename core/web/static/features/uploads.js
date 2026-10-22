import { showAppError } from "../lib/toasts.js";
import { getUploadLimits } from "../upload/upload_api.js";
import { Toast } from "./toast.js";
import { getUploadService, uploadService } from "./uploads/service.js";
import { getSessionUnlock, onSessionUnlock } from "./vault.js";

function formatBytes(bytes) {
	if (!bytes) return "0 B";
	const units = ["B", "KB", "MB", "GB", "TB"];
	const i = Math.floor(Math.log(bytes) / Math.log(1024));
	return (bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0) + " " + units[i];
}

function parseQueueLimit(value, fallback) {
	const parsed = Number.parseInt(String(value || ""), 10);
	if (!Number.isFinite(parsed)) return fallback;
	return parsed <= 0 ? 0 : parsed;
}

function parsePositiveInt(value, fallback) {
	const parsed = Number.parseInt(String(value || ""), 10);
	if (!Number.isFinite(parsed) || parsed <= 0) return fallback;
	return parsed;
}

export async function initUploads() {
	if (document.body.hasAttribute("data-uploads-ready")) return;
	document.body.setAttribute("data-uploads-ready", "true");

	const input = document.getElementById("upload-file");
	const browseFilesButton = document.getElementById("upload-browse-files");
	const abortButton = document.getElementById("upload-abort");
	const dropzone = document.getElementById("upload-dropzone");
	const confirmBackdrop = document.getElementById("upload-confirm-backdrop");
	const confirmStart = document.getElementById("upload-confirm-start");
	const confirmCancel = document.getElementById("upload-confirm-cancel");
	const metaTitle = document.getElementById("upload-meta-title");
	const metaDetail = document.getElementById("upload-meta-detail");
	const status = document.getElementById("upload-status");
	const queueList = document.getElementById("upload-queue-list");
	const queueMeta = document.getElementById("upload-queue-meta");
	const queueEmpty = document.getElementById("upload-queue-empty");
	const uploadControls = document.getElementById("upload-controls");
	const uploadChip = document.getElementById("upload-chip");
	const uploadChipName = document.getElementById("upload-chip-name");
	const uploadChipSize = document.getElementById("upload-chip-size");
	const uploadChipClear = document.getElementById("upload-chip-clear");
	const uploadIconBank = document.querySelector(".upload-icon-bank");

	if (!input || !queueList || !status) return;

	let rawLimits = null;
	try {
		rawLimits = await getUploadLimits();
	} catch (error) {
		setStatus(error && error.message ? error.message : "Failed to load upload limits.");
		showUploadErrorToast({ error: error && error.message ? error.message : "Failed to load upload limits." });
		return;
	}
	const uploadLimits = {
		maxQueueItems: parseQueueLimit(rawLimits && rawLimits.maxQueueItems, 300),
	};
	const uploadPolicy = {
		partConcurrency: parsePositiveInt(rawLimits && rawLimits.partConcurrency, 3),
	};
	const runner = getUploadService({ limits: uploadLimits, policy: uploadPolicy });
	window.uploadRunner = uploadService;
	let selectedFiles = [];
	let state = { jobs: [], activeJobId: null };
	const completedBatches = new Set();
	let renderScheduled = false;

	runner.onState(function (nextState) {
		state = nextState || state;
		scheduleRender();
	});
	runner.onEvent(function (event) {
		if (event && event.type === "error") {
			const message = friendlyUploadError(event);
			setStatus(message);
			showUploadErrorToast(event, message);
			return;
		}
		if (event && event.type === "batch-complete") {
			if (event.batchId && !completedBatches.has(event.batchId)) {
				completedBatches.add(event.batchId);
				const uploaded = event.total || 0;
				Toast.success(uploaded + " file(s) uploaded.", { title: "Upload complete" });
				const filesCount = document.querySelector(".summary-value-files");
				if (filesCount) {
					const current = Number.parseInt(filesCount.textContent.replace(/[^0-9]/g, ""), 10);
					if (Number.isFinite(current)) {
						filesCount.textContent = (current + uploaded).toLocaleString("en-US");
					}
				}
			}
		}
	});
	state = runner.getState();
	scheduleRender();

	onSessionUnlock(function (session) {
		runner.setVaultSession(session);
	});
	runner.setVaultSession(getSessionUnlock());

	browseFilesButton && browseFilesButton.addEventListener("click", function () { input.click(); });
	input.addEventListener("change", function () {
		if (!input.files || !input.files.length) return;
		selectedFiles = Array.from(input.files);
		showSelectedFiles(selectedFiles);
		showStartDialog(selectedFiles.length);
		input.value = "";
	});
	// Files may have been set on the input before this listener was bound
	// (the limits fetch above is async); the change event is lost but the
	// selection persists, so replay it now that the panel is ready.
	if (input.files && input.files.length) {
		selectedFiles = Array.from(input.files);
		showSelectedFiles(selectedFiles);
		showStartDialog(selectedFiles.length);
		input.value = "";
	}

	dropzone && dropzone.addEventListener("dragover", function (e) { e.preventDefault(); dropzone.classList.add("is-dragover"); });
	dropzone && dropzone.addEventListener("dragleave", function () { dropzone.classList.remove("is-dragover"); });
	dropzone && dropzone.addEventListener("drop", function (e) {
		e.preventDefault();
		dropzone.classList.remove("is-dragover");
		if (e.dataTransfer.files && e.dataTransfer.files.length) {
			selectedFiles = Array.from(e.dataTransfer.files);
			showSelectedFiles(selectedFiles);
			showStartDialog(selectedFiles.length);
		}
	});

	uploadChipClear && uploadChipClear.addEventListener("click", function () {
		selectedFiles = [];
		hideSelectedFiles();
	});

	confirmStart && confirmStart.addEventListener("click", function () {
		hideDialog();
		if (selectedFiles.length) {
			runner.addFiles(selectedFiles.slice()).catch(function (error) {
				setStatus(error && error.message ? error.message : "Upload failed.");
				if (typeof console !== "undefined" && console.error) console.error(error);
			});
			selectedFiles = [];
			hideSelectedFiles();
		}
	});

	confirmCancel && confirmCancel.addEventListener("click", function () {
		hideDialog();
		selectedFiles = [];
		hideSelectedFiles();
	});

	abortButton && abortButton.addEventListener("click", function () {
		runner.cancelAll().catch(function (error) {
			if (typeof console !== "undefined" && console.error) console.error(error);
		});
	});

	queueList.addEventListener("click", function (event) {
		const target = event.target;
		if (!target || !target.closest) return;
		const button = target.closest("[data-job-action]");
		if (!button) return;
		const jobId = button.getAttribute("data-job-id");
		const action = button.getAttribute("data-job-action");
		if (!jobId || !action) return;
		if (action === "cancel") runner.cancelJob(jobId).catch(function (error) { if (typeof console !== "undefined" && console.error) console.error(error); });
		if (action === "remove") runner.removeJob(jobId);
	});

	window.addEventListener("beforeunload", function (event) {
		if (!pageRetainsUploadData()) return;
		event.preventDefault();
		event.returnValue = "";
	});

	window.addEventListener("pagehide", function () {
		if (!pageRetainsUploadData()) return;
		selectedFiles = [];
		hideSelectedFiles();
		clearInputSelection();
		runner.cancelActiveUploadsBestEffort();
	});

	window.addEventListener("pageshow", function (event) {
		if (!event || !event.persisted) return;
		selectedFiles = [];
		hideSelectedFiles();
		clearInputSelection();
	});

	function setStatus(text) { status.textContent = text; }
	function friendlyUploadError(event) {
		switch (String((event && event.code) || "")) {
			case "storage_limit_exceeded":
				return "You're out of storage. Free up space or raise your limit, then try again.";
			case "unauthorized":
			case "forbidden":
				return "You don't have permission to upload. Please sign in and try again.";
			case "validation_failed":
				return "That file couldn't be accepted. Check the file and try again.";
			case "upload_not_found":
			case "not_found":
				return "The upload session expired. Please try the upload again.";
			case "conflict":
				return "This upload was already cancelled or is in progress. Please try again.";
			default:
				return "Upload failed. Please try again.";
		}
	}
	function showUploadErrorToast(event, message) {
		showAppError(event, {
			code: String((event && event.code) || "upload_failed"),
			message: message || "Upload failed",
		});
	}
	function hideDialog() { confirmBackdrop && confirmBackdrop.classList.add("is-hidden"); }
	function showDialog() { confirmBackdrop && confirmBackdrop.classList.remove("is-hidden"); }
	function showStartDialog(count) {
		const title = document.getElementById("upload-confirm-title");
		const body = document.getElementById("upload-confirm-meta");
		if (title) title.textContent = "Start upload?";
		if (body) body.textContent = count + " file(s) selected.";
		if (confirmStart) confirmStart.textContent = "Start upload";
		if (confirmCancel) confirmCancel.textContent = "Cancel";
		showDialog();
	}
	function showSelectedFiles(files) {
		if (!uploadChip || !uploadChipName || !uploadChipSize) return;
		uploadChipName.textContent = files.length === 1 ? files[0].name : files.length + " files";
		uploadChipSize.textContent = formatBytes(files.reduce(function (sum, file) { return sum + file.size; }, 0));
		uploadChip.classList.remove("is-hidden");
	}
	function hideSelectedFiles() {
		uploadChip && uploadChip.classList.add("is-hidden");
		if (uploadChipName) uploadChipName.textContent = "";
		if (uploadChipSize) uploadChipSize.textContent = "";
		clearInputSelection();
	}

	function clearInputSelection() {
		if (input) {
			input.value = "";
		}
	}

	function pageRetainsUploadData() {
		return selectedFiles.length > 0 || runner.hasActiveUploads();
	}


	function scheduleRender() {
		if (renderScheduled) return;
		renderScheduled = true;
		requestAnimationFrame(function () {
			renderScheduled = false;
			render();
		});
	}

	function render() {
		const jobs = (state.jobs || []).slice().sort(function (a, b) { return String(a.createdAt || "").localeCompare(String(b.createdAt || "")); });
		const existing = new Map();
		Array.from(queueList.children).forEach(function (node) {
			if (node && node.id) {
				existing.set(node.id, node);
			}
		});
		const hasCancelableJobs = jobs.some(function (job) {
			return job.status === "queued" || job.status === "running";
		});
		if (uploadControls) {
			uploadControls.classList.toggle("is-hidden", !hasCancelableJobs);
		}
		if (!jobs.length) {
			queueList.replaceChildren();
			queueEmpty && queueEmpty.classList.remove("is-hidden");
			queueMeta && (queueMeta.textContent = "0 items active");
			if (!state.activeJobId) setStatus("No uploads in progress.");
			return;
		}
		queueEmpty && queueEmpty.classList.add("is-hidden");
		let activeCount = 0;
		const fragment = document.createDocumentFragment();
		jobs.forEach(function (job) {
			if (job.status === "running") activeCount++;
			const id = "upload-task-" + job.jobId;
			const node = renderJob(job, existing.get(id) || null);
			existing.delete(id);
			fragment.appendChild(node);
		});
		existing.forEach(function (node) {
			if (node && node.parentNode === queueList) {
				node.parentNode.removeChild(node);
			}
		});
		queueList.replaceChildren(fragment);
		if (queueMeta) queueMeta.textContent = activeCount > 0 ? activeCount + " item" + (activeCount > 1 ? "s" : "") + " active" : jobs.length + " queued";
		const active = jobs.find(function (job) { return job.status === "running"; });
		if (active) {
			setStatus("Uploading " + active.fileName + ". Leaving this page will cancel it.");
		} else if (jobs.some(function (job) { return job.status === "queued"; })) {
			setStatus("Uploads queued. Leaving this page will cancel them.");
		}
	}

	function renderJob(job, existing) {
		const li = existing || document.createElement("li");
		if (!existing) {
			li.id = "upload-task-" + job.jobId;
			li.innerHTML = '<div class="queue-item-top"><div class="queue-item-file"><span class="queue-item-name"></span><span class="queue-item-badge"></span></div><div class="queue-item-actions"></div></div><div class="queue-item-track"><span class="queue-item-fill"></span></div><div class="queue-item-meta"><span class="queue-item-progress mono"></span><span class="queue-item-speed mono"></span></div>';
		}
		li.className = "queue-item" + (job.status === "completed" ? " is-complete" : job.status === "failed" || job.status === "canceled" ? " is-error" : "");
		const nameEl = li.querySelector(".queue-item-name");
		const badge = li.querySelector(".queue-item-badge");
		const actions = li.querySelector(".queue-item-actions");
		const fill = li.querySelector(".queue-item-fill");
		const progress = li.querySelector(".queue-item-progress");
		const detail = li.querySelector(".queue-item-speed");
		if (nameEl) nameEl.textContent = job.fileName;
		if (badge) {
			badge.className = "queue-item-badge is-" + job.status;
			badge.textContent = job.status;
		}
		if (actions) {
			const signature = actionSignature(job);
			if (actions.getAttribute("data-signature") !== signature) {
				actions.textContent = "";
				actions.setAttribute("data-signature", signature);
				buildActions(actions, job);
			}
		}
		if (fill) {
			fill.style.transition = "width 180ms linear";
			fill.style.width = job.fileSize > 0 ? Math.round(((job.completedBytes || 0) / job.fileSize) * 100) + "%" : "0%";
		}
		if (progress) {
			progress.textContent = formatBytes(job.completedBytes || 0) + " / " + formatBytes(job.fileSize);
		}
		if (detail) {
			detail.textContent = job.transferRate > 0 ? formatBytes(job.transferRate) + "/s" : job.status;
		}
		return li;
	}

	function actionSignature(job) {
		return String(job.status || "");
	}

	function buildActions(actions, job) {
		if (job.status === "queued" || job.status === "running") {
			actions.appendChild(actionButton(job.jobId, "cancel", "Cancel", "queue-item-action is-cancel", "trash"));
		} else if (job.status === "failed") {
			actions.appendChild(actionButton(job.jobId, "remove", "Remove", "queue-item-action is-cancel", "trash"));
		} else if (job.status === "completed" || job.status === "canceled") {
			actions.appendChild(actionButton(job.jobId, "remove", "Remove", "queue-item-action is-cancel", "trash"));
		}
	}

	function actionButton(jobId, action, label, className, iconName) {
		const button = document.createElement("button");
		button.type = "button";
		button.className = className;
		button.setAttribute("data-job-id", jobId);
		button.setAttribute("data-job-action", action);
		button.setAttribute("aria-label", label);
		button.title = label;
		const icon = uploadIconBank && uploadIconBank.querySelector('[data-upload-icon="' + (iconName || action) + '"] svg');
		if (icon) {
			button.appendChild(icon.cloneNode(true));
		}
		return button;
	}
}
