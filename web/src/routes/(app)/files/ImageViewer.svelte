<script lang="ts">
	import { onDestroy } from 'svelte';
	import 'photoswipe/style.css';
	import type { Content } from 'photoswipe/lightbox';
	import type { FileManagerEntry } from './types';
	import { readDecryptedFile } from '#lib/vault/read-file.ts';

	type PreviewRequest = { fileId: string; trigger: HTMLElement };
	type Props = {
		entries: FileManagerEntry[];
		request: PreviewRequest | null;
		onClose: () => void;
		onError: (message: string) => void;
	};
	type ContentResource = { controller: AbortController; objectUrl?: string };

	let { entries, request, onClose, onError }: Props = $props();
	let lightbox: import('photoswipe/lightbox').default | undefined;
	let loadingLightbox: Promise<import('photoswipe/lightbox').default> | undefined;
	let lastRequest: PreviewRequest | null = null;
	let openGeneration = 0;
	let destroyed = false;
	const contentResources = new Map<Content, ContentResource>();

	function ensureLightbox(): Promise<import('photoswipe/lightbox').default> {
		if (lightbox) return Promise.resolve(lightbox);
		if (loadingLightbox) return loadingLightbox;

		loadingLightbox = import('photoswipe/lightbox').then(({ default: PhotoSwipeLightbox }) => {
			const instance = new PhotoSwipeLightbox({
				pswpModule: () => import('photoswipe'),
				preload: [0, 1],
				showHideAnimationType: 'zoom'
			});
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

	async function loadImage(content: Content, entry: FileManagerEntry, image: HTMLImageElement): Promise<void> {
		const resource: ContentResource = { controller: new AbortController() };
		contentResources.set(content, resource);
		try {
			const blob = await readDecryptedFile(entry.id, entry.type, resource.controller.signal);
			if (resource.controller.signal.aborted) return;

			resource.objectUrl = URL.createObjectURL(blob);
			image.onload = () => {
				if (resource.controller.signal.aborted) return;
				content.onLoaded();
			};
			image.onerror = () => {
				if (resource.controller.signal.aborted) return;
				onError(`Could not preview ${entry.name}.`);
				content.onError();
			};
			image.src = resource.objectUrl;
		} catch (error) {
			if (resource.controller.signal.aborted) return;
			onError(error instanceof Error ? error.message : `Could not preview ${entry.name}.`);
			content.onError();
		}
	}

	function releaseContent(content: Content): void {
		const resource = contentResources.get(content);
		if (!resource) return;
		resource.controller.abort();
		if (resource.objectUrl) URL.revokeObjectURL(resource.objectUrl);
		contentResources.delete(content);
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
				const thumbnail = pending.trigger.querySelector('img') ?? pending.trigger;
				const dataSource = availableEntries.map((entry) => {
					const thumbnailWidth = entry.thumbnail?.width ?? 0;
					const thumbnailHeight = entry.thumbnail?.height ?? 0;
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
						msrc: entry.id === pending.fileId && thumbnail instanceof HTMLImageElement ? thumbnail.currentSrc : undefined
					};
				});
				const bounds = thumbnail.getBoundingClientRect();
				instance.loadAndOpen(index, dataSource, { x: bounds.left + bounds.width / 2, y: bounds.top + bounds.height / 2 });
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
