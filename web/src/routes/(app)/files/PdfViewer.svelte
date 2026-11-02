<script lang="ts">
	import { asset } from '$app/paths';
	import { onMount } from 'svelte';
	import { ChevronLeft, ChevronRight, Maximize2, Minimize2, Search, X, ZoomIn, ZoomOut } from '@lucide/svelte';
	import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url';
	import 'pdfjs-dist/web/pdf_viewer.css';
	import { DecryptedFileReader } from '#lib/vault/read-file.ts';
	import type { PDFDocumentLoadingTask } from 'pdfjs-dist';
	import type { EventBus } from 'pdfjs-dist/types/web/event_utils';
	import type { PDFFindController } from 'pdfjs-dist/types/web/pdf_find_controller';
	import type { PDFLinkService } from 'pdfjs-dist/types/web/pdf_link_service';
	import type { PDFViewer as PDFViewerInstance } from 'pdfjs-dist/types/web/pdf_viewer';
	import type { PdfRangeTransport } from '#lib/pdf/range-transport.ts';
	import type { FileManagerEntry } from './types';

	let { entry, onClose }: { entry: FileManagerEntry; onClose: () => void } = $props();
	let overlay = $state<HTMLDivElement>();
	let scrollContainer = $state<HTMLDivElement>();
	let pageElement = $state<HTMLDivElement>();
	let status = $state('Loading PDF…');
	let error = $state('');
	let currentPage = $state(1);
	let pageCount = $state(0);
	let pageInput = $state('1');
	let zoom = $state(100);
	let query = $state('');
	let matchCount = $state(0);
	let currentMatch = $state(0);
	let isFullscreen = $state(false);
	let pdfViewer = $state<PDFViewerInstance>();
	let eventBus: EventBus | undefined;
	let findController: PDFFindController | undefined;
	let linkService: PDFLinkService | undefined;
	let loadingTask: PDFDocumentLoadingTask | undefined;

	function closeTopmost(event: KeyboardEvent): void {
		if (event.key !== 'Escape') return;
		const overlays = document.querySelectorAll('[data-modal-open="true"]');
		if (overlays.item(overlays.length - 1) === overlay) onClose();
	}

	function changePage(page: number): void {
		if (!pdfViewer || !Number.isSafeInteger(page)) return;
		pdfViewer.currentPageNumber = Math.max(1, Math.min(pageCount, page));
		pageInput = String(pdfViewer.currentPageNumber);
	}

	function commitPage(): void {
		const page = Number(pageInput);
		if (!Number.isSafeInteger(page)) {
			pageInput = String(currentPage);
			return;
		}
		changePage(page);
	}

	function changeZoom(amount: number): void {
		if (!pdfViewer) return;
		pdfViewer.currentScale = Math.max(0.25, Math.min(4, pdfViewer.currentScale + amount));
		zoom = Math.round(pdfViewer.currentScale * 100);
	}

	function search(nextQuery: string, direction: 'next' | 'previous' = 'next'): void {
		query = nextQuery;
		if (!eventBus || !findController || !nextQuery.trim()) return;
		eventBus.dispatch('find', {
			source: findController,
			type: 'again',
			query: nextQuery,
			phraseSearch: true,
			caseSensitive: false,
			entireWord: false,
			highlightAll: true,
			findPrevious: direction === 'previous'
		});
	}

	function handleSearchInput(value: string): void {
		query = value;
		if (!eventBus || !findController) return;
		if (!value.trim()) {
			currentMatch = 0;
			matchCount = 0;
			eventBus.dispatch('findbarclose', { source: findController });
			return;
		}
		eventBus.dispatch('find', {
			source: findController,
			type: '',
			query: value,
			phraseSearch: true,
			caseSensitive: false,
			entireWord: false,
			highlightAll: true,
			findPrevious: false
		});
	}

	function toggleFullscreen(): void {
		if (!overlay) return;
		if (document.fullscreenElement) void document.exitFullscreen().catch(() => {});
		else void overlay.requestFullscreen().catch(() => {});
	}

	onMount(() => {
		const controller = new AbortController();
		const reader = new DecryptedFileReader(entry.id, controller.signal);
		let transport: PdfRangeTransport | undefined;
		let disposed = false;

		overlay?.focus();
		const updateFullscreen = () => isFullscreen = document.fullscreenElement === overlay;
		document.addEventListener('fullscreenchange', updateFullscreen);

		async function openPdf(): Promise<void> {
			try {
				const { plaintextSize } = await reader.load();
				if (controller.signal.aborted) return;

				const pdfjs = await import('pdfjs-dist');
				// PDF.js's viewer bundle reads its core API from this global during module evaluation.
				Reflect.set(globalThis, 'pdfjsLib', pdfjs);
				let viewerModule: typeof import('pdfjs-dist/web/pdf_viewer.mjs');
				try {
					viewerModule = await import('pdfjs-dist/web/pdf_viewer.mjs');
				} finally {
					Reflect.deleteProperty(globalThis, 'pdfjsLib');
				}
				const transportModule = await import('#lib/pdf/range-transport.ts');
				pdfjs.GlobalWorkerOptions.workerSrc = workerUrl;
				eventBus = new viewerModule.EventBus();
				linkService = new viewerModule.PDFLinkService({ eventBus });
				linkService.externalLinkEnabled = false;
				findController = new viewerModule.PDFFindController({ linkService, eventBus });
				// Treat PDF content as untrusted: no annotation layer, forms, or action scripting manager.
				pdfViewer = new viewerModule.PDFViewer({
					container: scrollContainer!,
					viewer: pageElement!,
					eventBus,
					linkService,
					findController,
					annotationMode: pdfjs.AnnotationMode.DISABLE,
					maxCanvasPixels: 8 * 1024 * 1024,
					enableDetailCanvas: false,
					imagesRightClickMinSize: -1,
					abortSignal: controller.signal
				} as ConstructorParameters<typeof viewerModule.PDFViewer>[0]);
				linkService.setViewer(pdfViewer);
				eventBus.on('pagesinit', () => {
					if (pdfViewer) {
						pdfViewer.currentScaleValue = 'page-width';
						zoom = Math.round(pdfViewer.currentScale * 100);
					}
					status = '';
				});
				eventBus.on('pagechanging', (event: { pageNumber: number }) => {
					currentPage = event.pageNumber;
					pageInput = String(event.pageNumber);
				});
				eventBus.on('updatefindmatchescount', (event: { matchesCount: { current: number; total: number } }) => {
					currentMatch = event.matchesCount.current;
					matchCount = event.matchesCount.total;
				});

				transport = new transportModule.PdfRangeTransport(plaintextSize, reader, (cause, begin, end) => {
					if (disposed) return;
					console.error('PDF preview could not read a byte range.', { fileId: entry.id, begin, end, cause });
					error = 'A PDF section could not be loaded. Close the preview and try again.';
					status = '';
					void loadingTask?.destroy().catch(() => {});
				});
				loadingTask = pdfjs.getDocument({
					range: transport,
					wasmUrl: asset('wasm/pdfjs/openjpeg.wasm').replace('openjpeg.wasm', ''),
					rangeChunkSize: 64 * 1024,
					disableAutoFetch: true,
					disableStream: true,
					enableXfa: false,
					maxImageSize: 16 * 1024 * 1024
				});
				const document = await loadingTask.promise;
				if (controller.signal.aborted || !pdfViewer) return;
				pageCount = document.numPages;
				pdfViewer.setDocument(document);
				linkService.setDocument(document);
			} catch (cause) {
				if (controller.signal.aborted) return;
				console.error('PDF preview failed to open.', { fileId: entry.id, cause });
				error = 'Could not open this PDF. It may be damaged, password-protected, or unavailable.';
				status = '';
			}
		}

		void openPdf();
		return () => {
			disposed = true;
			controller.abort();
			document.removeEventListener('fullscreenchange', updateFullscreen);
			transport?.abort();
			pdfViewer?.setDocument(null);
			linkService?.setDocument(null);
			void (async () => {
				try {
					await loadingTask?.destroy();
				} catch {
					// PDF.js may already have destroyed a failed or canceled loading task.
				}
				await reader.dispose();
			})();
		};
	});
</script>

<svelte:window onkeydown={closeTopmost} />

<div class="fixed inset-0 z-50 flex flex-col bg-(--bg)" data-modal-open="true" role="dialog" aria-modal="true" aria-label={`PDF preview: ${entry.name}`} tabindex="-1" bind:this={overlay}>
	<header class="flex min-h-14 flex-wrap items-center gap-2 border-b border-(--border) bg-(--surface) px-3 py-1.5 text-sm">
		<strong class="min-w-0 flex-1 basis-32 truncate" title={entry.name}>{entry.name}</strong>
		<div class="flex items-center gap-1">
			<button class="viewer-button" type="button" aria-label="Previous page" title="Previous page" disabled={currentPage <= 1} onclick={() => changePage(currentPage - 1)}><ChevronLeft class="size-4" /></button>
			<form class="flex items-center gap-1" onsubmit={(event) => { event.preventDefault(); commitPage(); }}>
				<input class="viewer-input w-12 text-center" aria-label="Page number" bind:value={pageInput} onblur={commitPage} />
				<span class="text-(--muted)">/ {pageCount || '—'}</span>
			</form>
			<button class="viewer-button" type="button" aria-label="Next page" title="Next page" disabled={!pageCount || currentPage >= pageCount} onclick={() => changePage(currentPage + 1)}><ChevronRight class="size-4" /></button>
		</div>
		<div class="hidden items-center gap-1 min-[760px]:flex">
			<button class="viewer-button" type="button" aria-label="Zoom out" title="Zoom out" disabled={!pdfViewer} onclick={() => changeZoom(-0.1)}><ZoomOut class="size-4" /></button>
			<span class="w-12 text-center tabular-nums text-(--muted)">{zoom}%</span>
			<button class="viewer-button" type="button" aria-label="Zoom in" title="Zoom in" disabled={!pdfViewer} onclick={() => changeZoom(0.1)}><ZoomIn class="size-4" /></button>
		</div>
		<form class="flex min-w-32 flex-1 items-center gap-1 rounded border border-(--border) px-2" onsubmit={(event) => { event.preventDefault(); search(query); }}>
			<Search class="size-4 text-(--muted)" aria-hidden="true" />
			<input class="viewer-search min-w-0 flex-1" aria-label="Search PDF text" placeholder="Find in PDF" bind:value={query} oninput={(event) => handleSearchInput(event.currentTarget.value)} />
			{#if query}<span class="min-w-10 text-center text-xs text-(--muted)">{currentMatch}/{matchCount}</span>{/if}
		</form>
		<button class="viewer-button" type="button" aria-label="Find next" title="Find next" disabled={!query.trim()} onclick={() => search(query, 'next')}><ChevronRight class="size-4" /></button>
		<button class="viewer-button" type="button" aria-label="Find previous" title="Find previous" disabled={!query.trim()} onclick={() => search(query, 'previous')}><ChevronLeft class="size-4" /></button>
		<button class="viewer-button min-w-12 px-1" type="button" aria-label="Fit page width" title="Fit page width" disabled={!pdfViewer} onclick={() => { if (pdfViewer) { pdfViewer.currentScaleValue = 'page-width'; zoom = Math.round(pdfViewer.currentScale * 100); } }}>Fit</button>
		<button class="viewer-button" type="button" aria-label={isFullscreen ? 'Exit full screen' : 'Enter full screen'} title={isFullscreen ? 'Exit full screen' : 'Full screen'} onclick={toggleFullscreen}>{#if isFullscreen}<Minimize2 class="size-4" />{:else}<Maximize2 class="size-4" />{/if}</button>
		<button class="viewer-button" type="button" aria-label="Close PDF preview" title="Close" onclick={onClose}><X class="size-4" /></button>
	</header>
	<div class="pdf-stage">
		<div class="pdf-scroll" bind:this={scrollContainer}>
			<div class="pdfViewer" bind:this={pageElement}></div>
		</div>
		{#if status}<div class="absolute inset-0 grid place-items-center bg-(--bg) text-(--muted)" role="status">{status}</div>{/if}
		{#if error}<div class="absolute inset-0 grid place-items-center bg-(--bg) p-6 text-center text-red-400" role="alert">{error}</div>{/if}
	</div>
</div>

<style>
	.pdf-stage { position: relative; min-height: 0; flex: 1; }
	.pdf-scroll { position: absolute; inset: 0; overflow: auto; background: var(--bg); }
	.viewer-button { display: inline-flex; width: 2rem; height: 2rem; cursor: pointer; align-items: center; justify-content: center; border: 1px solid transparent; border-radius: 0.375rem; color: var(--text); }
	.viewer-button:hover:not(:disabled) { background: var(--surface-highest); }
	.viewer-button:focus-visible, .viewer-input:focus-visible, .viewer-search:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
	.viewer-button:disabled { cursor: not-allowed; opacity: 0.4; }
	.viewer-input, .viewer-search { height: 2rem; border: 0; background: transparent; color: var(--text); outline: none; }
	.viewer-search { width: 8rem; }
	:global(.pdfViewer) { padding: 1rem 0; }
	:global(.pdfViewer .page) { margin: 0 auto 1rem; border: 1px solid var(--border); }
	:global(.pdfViewer .textLayer) { user-select: text; }
	@media (max-width: 760px) {
		.viewer-button { width: 1.8rem; height: 1.8rem; }
	}
</style>
