<script lang="ts">
	import { onDestroy } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import 'photoswipe/style.css';
	import type { Content } from 'photoswipe/lightbox';
	import { readDecryptedThumbnail } from '#lib/vault/thumbnail.ts';
	import type { FileManagerEntry } from './types';
	import { readDecryptedFile } from '#lib/vault/read-file.ts';
	import { isHeicImage } from '#lib/utils/image.ts';
	import { decodeHeicImage } from '#lib/utils/heic.ts';

	type PreviewRequest = { fileId: string; trigger?: HTMLElement };
	type Props = {
		entries: FileManagerEntry[];
		request: PreviewRequest | null;
		onClose: () => void;
		onError: (message: string) => void;
	};
	type ContentResource = {
		controller: AbortController;
		objectUrl?: string;
		pendingObjectUrl?: string;
		status: 'loading-thumbnail' | 'ready' | 'loading-original' | 'original' | 'failed';
	};

	let { entries, request, onClose, onError }: Props = $props();
	let lightbox: import('photoswipe/lightbox').default | undefined;
	let loadingLightbox: Promise<import('photoswipe/lightbox').default> | undefined;
	let lastRequest: PreviewRequest | null = null;
	let openGeneration = 0;
	let destroyed = false;
	let originalButton: HTMLButtonElement | undefined;
	const contentResources = new SvelteMap<Content, ContentResource>();

	function updateOriginalButton(): void {
		const content = lightbox?.pswp?.currSlide?.content;
		const resource = content && contentResources.get(content);

		if (!originalButton) return;
		originalButton.hidden = resource?.status === 'original';
		originalButton.disabled =
			!resource ||
			resource.status === 'loading-thumbnail' ||
			resource.status === 'loading-original';
		originalButton.textContent =
			resource?.status === 'loading-original'
				? 'Loading…'
				: resource?.status === 'failed'
					? 'Retry original'
					: 'View original';
	}

	function ensureLightbox(): Promise<import('photoswipe/lightbox').default> {
		if (lightbox) return Promise.resolve(lightbox);
		if (loadingLightbox) return loadingLightbox;

		loadingLightbox = import('photoswipe/lightbox').then(({ default: PhotoSwipeLightbox }) => {
			const instance = new PhotoSwipeLightbox({
				pswpModule: () => import('photoswipe'),
				preload: [0, 1],
				showHideAnimationType: 'zoom',
			});
			instance.on('uiRegister', () => {
				instance.pswp?.ui?.registerElement({
					name: 'viewOriginal',
					className: 'arkive-view-original',
					html: 'View original',
					ariaLabel: 'View original image',
					isButton: true,
					order: 15,
					onInit: (element) => {
						originalButton = element as HTMLButtonElement;
						updateOriginalButton();
					},
					onClick: (_event, _element, pswp) => {
						const content = pswp.currSlide?.content;

						if (content) void loadOriginal(content);
					},
				});
			});
			instance.on('change', updateOriginalButton);
			instance.on('contentLoadImage', (event) => {
				event.preventDefault();
				const { content } = event;
				const entry = entries.find((candidate) => candidate.id === content.data.id);
				const image = content.element;

				if (!entry || !(image instanceof HTMLImageElement)) {
					content.onError();

					return;
				}

				// Replacing this element drops PhotoSwipe's dimensions and desynchronizes pan/zoom.
				image.alt = entry.name;
				content.state = 'loading';
				void loadImage(content, entry, image);
			});
			instance.on('contentDestroy', ({ content }) => releaseContent(content));
			instance.on('close', onClose);
			instance.init();
			if (destroyed) {
				instance.destroy();

				return instance;
			}
			lightbox = instance;

			return instance;
		});

		return loadingLightbox;
	}

	async function loadImage(
		content: Content,
		entry: FileManagerEntry,
		image: HTMLImageElement,
	): Promise<void> {
		const resource: ContentResource = {
			controller: new AbortController(),
			status: entry.thumbnail ? 'loading-thumbnail' : 'ready',
		};
		contentResources.set(content, resource);
		image.style.visibility = 'hidden';
		updateOriginalButton();
		try {
			const blob = entry.thumbnail
				? await readDecryptedThumbnail(
						{ id: entry.id, ...entry.thumbnail },
						resource.controller.signal,
					)
				: null;

			if (resource.controller.signal.aborted) return;
			if (!blob) {
				resource.status = 'ready';
				content.onLoaded();
				updateOriginalButton();

				return;
			}

			resource.objectUrl = URL.createObjectURL(blob);
			image.onload = () => {
				if (resource.controller.signal.aborted) return;
				resource.status = 'ready';
				image.style.visibility = 'visible';
				content.onLoaded();
				updateOriginalButton();
			};
			image.onerror = () => {
				if (resource.controller.signal.aborted) return;
				resource.status = 'ready';
				image.style.visibility = 'hidden';
				if (resource.objectUrl) URL.revokeObjectURL(resource.objectUrl);
				resource.objectUrl = undefined;
				content.onLoaded();
				updateOriginalButton();
			};
			image.src = resource.objectUrl;
			updateOriginalButton();
		} catch {
			if (resource.controller.signal.aborted) return;
			resource.status = 'ready';
			content.onLoaded();
			updateOriginalButton();
		}
	}

	async function loadOriginal(content: Content): Promise<void> {
		const entry = entries.find((candidate) => candidate.id === content.data.id);
		const resource = contentResources.get(content);
		const image = content.element;

		if (
			!entry ||
			!resource ||
			!(image instanceof HTMLImageElement) ||
			resource.status === 'loading-thumbnail' ||
			resource.status === 'loading-original' ||
			resource.status === 'original'
		)
			return;

		resource.status = 'loading-original';
		updateOriginalButton();
		try {
			const decrypted = await readDecryptedFile(entry.id, entry.type, resource.controller.signal);

			if (resource.controller.signal.aborted) return;

			const fail = (error?: unknown) => {
				resource.status = 'failed';
				if (resource.objectUrl) {
					image.src = resource.objectUrl;
					image.style.visibility = 'visible';
				} else {
					image.style.visibility = 'hidden';
				}
				onError(error instanceof Error ? error.message : `Could not preview ${entry.name}.`);
				updateOriginalButton();
			};
			const showOriginal = (blob: Blob, usingFallback = false) => {
				const originalUrl = URL.createObjectURL(blob);
				resource.pendingObjectUrl = originalUrl;
				image.onload = () => {
					if (resource.controller.signal.aborted) return;
					const thumbnailUrl = resource.objectUrl;
					resource.objectUrl = originalUrl;
					resource.pendingObjectUrl = undefined;
					resource.status = 'original';
					image.style.visibility = 'visible';
					image.onload = null;
					image.onerror = null;
					if (thumbnailUrl) URL.revokeObjectURL(thumbnailUrl);
					updateOriginalButton();
				};
				image.onerror = () => {
					if (resource.controller.signal.aborted) return;
					image.onload = null;
					image.onerror = null;
					URL.revokeObjectURL(originalUrl);
					resource.pendingObjectUrl = undefined;
					if (!usingFallback && isHeicImage(entry.name, entry.type)) {
						image.style.visibility = 'hidden';
						void decodeHeicImage(
							decrypted,
							entry.name,
							entry.type,
							null,
							resource.controller.signal,
						)
							.then(({ blob: decoded }) => {
								if (!resource.controller.signal.aborted) showOriginal(decoded, true);
							})
							.catch(fail);

						return;
					}
					fail();
				};
				image.src = originalUrl;
			};
			showOriginal(decrypted);
		} catch (error) {
			if (resource.controller.signal.aborted) return;
			resource.status = 'failed';
			onError(error instanceof Error ? error.message : `Could not preview ${entry.name}.`);
			updateOriginalButton();
		}
	}

	function releaseContent(content: Content): void {
		const resource = contentResources.get(content);

		if (!resource) return;
		resource.controller.abort();
		if (resource.objectUrl) URL.revokeObjectURL(resource.objectUrl);
		if (resource.pendingObjectUrl) URL.revokeObjectURL(resource.pendingObjectUrl);
		contentResources.delete(content);
		updateOriginalButton();
	}

	$effect(() => {
		const pending = request;
		const availableEntries = entries;

		if (!pending) {
			lastRequest = null;

			return;
		}
		if (pending === lastRequest) return;
		lastRequest = pending;
		const generation = ++openGeneration;
		void (async () => {
			try {
				const instance = await ensureLightbox();

				if (generation !== openGeneration) return;
				const index = availableEntries.findIndex((entry) => entry.id === pending.fileId);

				if (index < 0) {
					onClose();

					return;
				}
				const thumbnail = pending.trigger?.querySelector('img') ?? pending.trigger;
				const dataSource = availableEntries.map((entry) => {
					const thumbnailWidth = entry.thumbnail?.thumbnailWidth ?? 0;
					const thumbnailHeight = entry.thumbnail?.thumbnailHeight ?? 0;
					const hasThumbnailSize = thumbnailWidth > 0 && thumbnailHeight > 0;
					const width = hasThumbnailSize ? thumbnailWidth : 4;
					const height = hasThumbnailSize ? thumbnailHeight : 3;
					// Thumbnails retain aspect ratio but are too small to size the full-resolution viewer.
					const scale = 4000 / Math.max(width, height);

					return {
						id: entry.id,
						src: entry.id,
						type: 'image',
						element: entry.id === pending.fileId ? thumbnail : undefined,
						width: Math.round(width * scale),
						height: Math.round(height * scale),
						alt: entry.name,
						msrc:
							entry.id === pending.fileId && thumbnail instanceof HTMLImageElement
								? thumbnail.currentSrc
								: undefined,
					};
				});
				const bounds = thumbnail?.getBoundingClientRect();
				instance.loadAndOpen(
					index,
					dataSource,
					bounds
						? { x: bounds.left + bounds.width / 2, y: bounds.top + bounds.height / 2 }
						: { x: window.innerWidth / 2, y: window.innerHeight / 2 },
				);
			} catch (error) {
				onError(error instanceof Error ? error.message : 'Could not open image preview.');
				onClose();
			}
		})();
	});

	onDestroy(() => {
		destroyed = true;
		openGeneration += 1;
		lightbox?.destroy();
		for (const content of contentResources.keys()) releaseContent(content);
	});
</script>

<style>
	:global(.pswp__button.arkive-view-original) {
		width: auto;
		padding-inline: 12px;
		color: white;
		font-size: 13px;
		font-weight: 600;
		white-space: nowrap;
	}

	:global(.pswp__button.arkive-view-original[hidden]) {
		display: none;
	}
</style>
