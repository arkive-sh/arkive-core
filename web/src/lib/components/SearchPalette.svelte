<script lang="ts">
	import { tick } from 'svelte';
	import { Search } from '@lucide/svelte';
	import Modal from '#lib/components/Modal.svelte';
	import { apiRequest } from '#lib/api.ts';
	import { getCachedThumbnail } from '#lib/thumbnail-cache.ts';
	import {
		loadSearchVault,
		type EncryptedSearchFile,
		type EncryptedSearchFolder,
		type SearchPreview
	} from '#lib/vault.ts';

	type Category = 'results' | 'shares' | 'settings';
	type SearchResult = {
		id: string;
		kind: string;
		title: string;
		meta?: string;
		status?: string;
		url: string;
		category: Category;
		score?: number;
		preview?: SearchPreview;
		file?: EncryptedSearchFile;
		thumbnailUrl?: string;
	};
	type SearchAPIItem = Omit<SearchResult, 'category' | 'score' | 'preview' | 'file' | 'thumbnailUrl'>;
	type SearchResponse = {
		results?: {
			files?: EncryptedSearchFile[];
			folders?: EncryptedSearchFolder[];
			shares?: SearchAPIItem[];
		};
	};
	type Props = { open: boolean; vaultId: string; onOpen: () => void; onClose: () => void };

	const SETTINGS = [
		{ id: 'settings-0', title: 'Instance Overview', terms: ['instance overview', 'instance', 'admin', 'email', 'storage', 'usage'], url: '/settings#settings-account', meta: 'Settings' },
		{ id: 'settings-1', title: 'Storage Provider', terms: ['storage provider', 'storage configuration', 'provider', 'local', 's3'], url: '/settings#settings-provider', meta: 'Settings' },
		{ id: 'settings-2', title: 'Security', terms: ['security', 'authentication', 'session', 'hardening'], url: '/settings#settings-security', meta: 'Settings' }
	];

	let { open, vaultId, onOpen, onClose }: Props = $props();
	let query = $state('');
	let results = $state<SearchResult[]>([]);
	let loading = $state(false);
	let hasSearched = $state(false);
	let activeIndex = $state(0);
	let searchInput: HTMLInputElement | undefined;
	let searchTimer: number | undefined;
	let abortController: AbortController | undefined;
	let latestQuery = '';
	const thumbnailURLs = new Set<string>();
	let fileFolderResults = $derived(results.filter((result) => result.category === 'results'));
	let shareResults = $derived(results.filter((result) => result.category === 'shares'));
	let settingsResults = $derived(results.filter((result) => result.category === 'settings'));

	function normalizeQuery(value: string) {
		return value.toLowerCase().normalize('NFKD').replace(/[^\p{L}\p{N}.]+/gu, ' ').trim();
	}

	function searchSettings(query: string): SearchResult[] {
		const normalized = normalizeQuery(query);
		if (!normalized) return [];
		return SETTINGS.filter((setting) => setting.terms.some((term) => normalizeQuery(term).includes(normalized))).map((setting) => ({
			id: setting.id,
			kind: 'setting',
			title: setting.title,
			meta: setting.meta,
			url: setting.url,
			category: 'settings'
		}));
	}

	function clearThumbnails() {
		thumbnailURLs.forEach((url) => URL.revokeObjectURL(url));
		thumbnailURLs.clear();
	}

	async function hydrateThumbnails(items: SearchResult[], query: string, signal: AbortSignal) {
		const vault = await loadSearchVault();
		for (const item of items) {
			const preview = item.preview;
			const file = item.file;
			const version = Number(preview?.thumbnail_version || 0);
			const size = Number(preview?.thumbnail_size || 0);
			if (!file || !preview?.has_thumbnail || version <= 0 || size <= 0 || signal.aborted) continue;

			try {
				const bytes = await getCachedThumbnail(file.id, version, size);
				if (!bytes?.length || signal.aborted) continue;
				const { thumbnailBytes } = await vault.decryptSearchThumbnail(file, bytes);
				if (!thumbnailBytes.length || signal.aborted) continue;
				const url = URL.createObjectURL(new Blob([Uint8Array.from(thumbnailBytes)], { type: preview.thumbnail_mime || 'image/webp' }));
				if (latestQuery !== query) {
					URL.revokeObjectURL(url);
					continue;
				}
				thumbnailURLs.add(url);
				results = results.map((result) => result.id === item.id ? { ...result, thumbnailUrl: url } : result);
			} catch {
				// An unavailable cached preview does not block text search results.
			}
		}
	}

	async function runSearch(query: string) {
		const settings = searchSettings(query);
		const controller = new AbortController();
		abortController?.abort();
		abortController = controller;
		loading = true;

		try {
			const vault = await loadSearchVault();
			const tokens = await vault.createSearchTokens(query, vaultId);
			if (!tokens.length) {
				if (latestQuery === query) {
					results = settings;
					hasSearched = true;
				}
				return;
			}

			const data = await apiRequest<SearchResponse>('/api/search', {
				method: 'POST',
				signal: controller.signal,
				headers: { Accept: 'application/json', 'Content-Type': 'application/json' },
				data: { tokens, limit: 20 }
			}, 'Search failed');
			if (controller.signal.aborted || latestQuery !== query) return;

			const folders = await Promise.all((data.results?.folders ?? []).map(async (folder): Promise<SearchResult | null> => {
				try {
					const opened = await vault.decryptSearchFolder(folder);
					return {
						id: folder.id,
						kind: 'folder',
						title: opened.name?.name || opened.metadata?.name || 'Encrypted folder',
						meta: 'Folder',
						score: Number(folder.score || 0),
						url: folder.url,
						category: 'results'
					};
				} catch {
					return null;
				}
			}));
			const files = await Promise.all((data.results?.files ?? []).map(async (file): Promise<SearchResult | null> => {
				try {
					const opened = await vault.decryptSearchFile(file);
					return {
						id: file.id,
						kind: 'file',
						title: opened.metadata?.name || 'Encrypted file',
						meta: opened.metadata?.mime || 'Encrypted',
						score: Number(file.score || 0),
						preview: opened.metadata?.preview,
						file,
						url: file.url,
						category: 'results'
					};
				} catch {
					return null;
				}
			}));
			if (controller.signal.aborted || latestQuery !== query) return;

			const ranked = [...folders, ...files]
				.filter((result): result is SearchResult => result !== null)
				.sort((a, b) => (b.score || 0) - (a.score || 0) || a.title.localeCompare(b.title));
			const shares = (data.results?.shares ?? []).map((result) => ({ ...result, category: 'shares' as const }));
			results = [...ranked, ...shares, ...settings];
			activeIndex = 0;
			hasSearched = true;
			void hydrateThumbnails(ranked, query, controller.signal);
		} catch {
			if (!controller.signal.aborted && latestQuery === query) {
				results = settings;
				hasSearched = true;
			}
		} finally {
			if (!controller.signal.aborted && latestQuery === query) loading = false;
		}
	}

	function scheduleSearch(event: Event) {
		query = (event.currentTarget as HTMLInputElement).value;
		const value = query.trim();
		latestQuery = value;
		if (searchTimer !== undefined) window.clearTimeout(searchTimer);
		abortController?.abort();
		clearThumbnails();
		results = searchSettings(value);
		activeIndex = 0;
		hasSearched = false;
		loading = false;
		if (!value) return;
		searchTimer = window.setTimeout(() => {
			searchTimer = undefined;
			void runSearch(value);
		}, 300);
	}

	function handleInputKeydown(event: KeyboardEvent) {
		if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
			if (!results.length) return;
			event.preventDefault();
			activeIndex = (activeIndex + (event.key === 'ArrowDown' ? 1 : -1) + results.length) % results.length;
			void tick().then(() => document.getElementById(`search-result-${activeIndex}`)?.scrollIntoView({ block: 'nearest' }));
		} else if (event.key === 'Enter' && results[activeIndex]) {
			event.preventDefault();
			const result = results[activeIndex];
			onClose();
			window.location.assign(result.url);
		}
	}

	function handleGlobalKeydown(event: KeyboardEvent) {
		if (open || event.key !== '/' || event.altKey || event.ctrlKey || event.metaKey) return;
		if (event.target instanceof HTMLElement && event.target.closest('input, textarea, [contenteditable="true"]')) return;
		event.preventDefault();
		onOpen();
	}

	$effect(() => {
		if (!open) return;
		query = '';
		results = [];
		loading = false;
		hasSearched = false;
		activeIndex = 0;
		latestQuery = '';
		clearThumbnails();
		let frame = 0;
		void tick().then(() => {
			frame = window.requestAnimationFrame(() => {
				searchInput?.focus();
				searchInput?.setSelectionRange(searchInput.value.length, searchInput.value.length);
			});
		});
		return () => {
			window.cancelAnimationFrame(frame);
			if (searchTimer !== undefined) window.clearTimeout(searchTimer);
			abortController?.abort();
			latestQuery = '';
			clearThumbnails();
		};
	});
</script>

<svelte:window onkeydown={handleGlobalKeydown} />

<Modal
	{open}
	label="Search vault"
	onClose={onClose}
	backdropClass="items-start! bg-[rgba(11,15,20,0.8)]! px-6! pt-18! pb-6! backdrop-blur-sm max-[640px]:items-start! max-[720px]:px-2.5! max-[720px]:pt-24! max-[720px]:pb-2.5!"
	dialogClass="w-[min(952px,100%)]! rounded-xl! bg-[rgba(27,32,36,0.96)]! shadow-[0_28px_60px_rgba(0,0,0,0.42)]! backdrop-blur-xl!"
	bodyClass="gap-0! p-0!"
	actionsClass="justify-start! gap-4! bg-[rgba(23,28,32,0.82)]! px-5.5! py-3! max-[720px]:flex-wrap"
>
	{#snippet header()}
		<div class="flex items-center gap-4 border-b border-(--border) bg-[rgba(37,43,46,0.72)] px-5.5 py-4.5 max-[720px]:px-4 max-[720px]:py-3.5">
			<Search class="size-5.5 shrink-0 stroke-2 text-[#c4e7ff]" aria-hidden="true" />
			<input
				bind:this={searchInput}
				id="search-panel-input"
				class="w-full border-0 bg-transparent text-lg leading-6 text-(--text) outline-none placeholder:text-(--muted)"
				type="search"
				placeholder="Search files, shares, or settings..."
				aria-label="Search files, shares, or settings"
				autocomplete="off"
				value={query}
				oninput={scheduleSearch}
				onkeydown={handleInputKeydown}
			/>
			<kbd class="inline-flex h-9 min-w-10 shrink-0 items-center justify-center rounded border border-(--border) bg-[rgba(48,53,57,0.85)] px-2.5 text-xs text-(--muted)">ESC</kbd>
		</div>
	{/snippet}

	<div class="grid max-h-[min(60vh,560px)] gap-4 overflow-y-auto p-3" aria-label="Search results">
		{#if fileFolderResults.length}
			<section class="grid gap-2">
				<h2 class="px-2.5 text-[11px] leading-4 font-semibold tracking-wider text-[#9eb8ce] uppercase">Results</h2>
				<div class="grid gap-0.5">
					{#each fileFolderResults as result (result.kind + ':' + result.id)}
						{@const index = results.indexOf(result)}
						<a
							id={`search-result-${index}`}
							href={result.url}
							class={`group flex items-center justify-between gap-4 rounded-md px-3.5 py-3.5 ${index === activeIndex ? 'bg-(--accent) text-(--accent-ink)' : 'text-(--text) hover:bg-(--accent) hover:text-(--accent-ink)'}`}
							onclick={onClose}
						>
							<span class="flex min-w-0 items-center gap-3.5">
								<span class="inline-flex size-8 shrink-0 items-center justify-center overflow-hidden rounded border border-[rgba(62,72,79,0.85)] bg-[rgba(48,53,57,0.42)] text-xs uppercase group-hover:text-(--accent-ink)">
									{#if result.thumbnailUrl}
										<img class="size-full object-cover" src={result.thumbnailUrl} alt={result.title} loading="lazy" decoding="async" />
									{:else}{result.kind === 'file' ? 'F' : 'D'}{/if}
								</span>
								<span class="grid min-w-0 gap-1">
									<span class="truncate text-[15px] leading-5">{result.title}</span>
									{#if result.meta}<span class="truncate text-[13px] leading-4 text-(--muted) group-hover:text-[rgba(0,53,74,0.78)]">{result.meta}</span>{/if}
								</span>
							</span>
							{#if result.status}<span class="text-xs font-bold text-(--muted) uppercase group-hover:text-[rgba(0,53,74,0.78)]">{result.status}</span>{/if}
						</a>
					{/each}
				</div>
			</section>
		{/if}
		{#if shareResults.length}
			<section class="grid gap-2">
				<h2 class="px-2.5 text-[11px] leading-4 font-semibold tracking-wider text-[#9eb8ce] uppercase">Shares</h2>
				<div class="grid gap-0.5">
					{#each shareResults as result (result.kind + ':' + result.id)}
						{@const index = results.indexOf(result)}
						<a id={`search-result-${index}`} href={result.url} class={`group flex items-center justify-between gap-4 rounded-md px-3.5 py-3.5 ${index === activeIndex ? 'bg-(--accent) text-(--accent-ink)' : 'text-(--text) hover:bg-(--accent) hover:text-(--accent-ink)'}`} onclick={onClose}>
							<span class="flex min-w-0 items-center gap-3.5"><span class="inline-flex size-8 shrink-0 items-center justify-center rounded border border-[rgba(62,72,79,0.85)] bg-[rgba(48,53,57,0.42)] text-xs uppercase group-hover:text-(--accent-ink)">S</span><span class="grid min-w-0 gap-1"><span class="truncate text-[15px] leading-5">{result.title}</span>{#if result.meta}<span class="truncate text-[13px] leading-4 text-(--muted) group-hover:text-[rgba(0,53,74,0.78)]">{result.meta}</span>{/if}</span></span>
							{#if result.status}<span class="text-xs font-bold text-(--muted) uppercase group-hover:text-[rgba(0,53,74,0.78)]">{result.status}</span>{/if}
						</a>
					{/each}
				</div>
			</section>
		{/if}
		{#if settingsResults.length}
			<section class="grid gap-2">
				<h2 class="px-2.5 text-[11px] leading-4 font-semibold tracking-wider text-[#9eb8ce] uppercase">Settings</h2>
				<div class="grid gap-0.5">
					{#each settingsResults as result (result.id)}
						{@const index = results.indexOf(result)}
						<a id={`search-result-${index}`} href={result.url} class={`group flex items-center justify-between gap-4 rounded-md px-3.5 py-3.5 ${index === activeIndex ? 'bg-(--accent) text-(--accent-ink)' : 'text-(--text) hover:bg-(--accent) hover:text-(--accent-ink)'}`} onclick={onClose}>
							<span class="flex min-w-0 items-center gap-3.5"><span class="inline-flex size-8 shrink-0 items-center justify-center rounded border border-[rgba(62,72,79,0.85)] bg-[rgba(48,53,57,0.42)] text-xs uppercase group-hover:text-(--accent-ink)">C</span><span class="grid min-w-0 gap-1"><span class="truncate text-[15px] leading-5">{result.title}</span>{#if result.meta}<span class="truncate text-[13px] leading-4 text-(--muted) group-hover:text-[rgba(0,53,74,0.78)]">{result.meta}</span>{/if}</span></span>
							{#if result.status}<span class="text-xs font-bold text-(--muted) uppercase group-hover:text-[rgba(0,53,74,0.78)]">{result.status}</span>{/if}
						</a>
					{/each}
				</div>
			</section>
		{/if}
		{#if loading}<div class="px-3.5 py-3 text-[13px] leading-4.5 text-[#9eb8ce]">Searching...</div>{/if}
		{#if hasSearched && !loading && !results.length}<div class="px-3.5 py-3 text-[13px] leading-4.5 text-(--muted)">No results found.</div>{/if}
	</div>

	{#snippet actions()}
		<div class="inline-flex items-center gap-2 text-[13px] leading-4.5 text-(--muted)"><kbd class="inline-flex h-6 min-w-6 items-center justify-center rounded border border-(--border) bg-[rgba(48,53,57,0.85)] px-1.5 text-xs">↑↓</kbd><span>to navigate</span></div>
		<div class="inline-flex items-center gap-2 text-[13px] leading-4.5 text-(--muted)"><kbd class="inline-flex h-6 min-w-6 items-center justify-center rounded border border-(--border) bg-[rgba(48,53,57,0.85)] px-1.5 text-xs">↵</kbd><span>to select</span></div>
	{/snippet}
</Modal>
