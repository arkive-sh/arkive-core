<script lang="ts">
	import { tick } from 'svelte';
	import { goto } from '$app/navigation';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { Search } from '@lucide/svelte';
	import Modal from '#lib/components/Modal.svelte';
	import { api } from '#lib/api.ts';
	import { getCachedThumbnail } from '#lib/thumbnail-cache.ts';
	import { fileLocationHref } from '#lib/utils/navigation.ts';
	import type { EncryptedSearchFile, EncryptedSearchFolder, SearchPreview } from '#lib/vault/types.ts';
	import { decryptSearchFile, decryptSearchFolder, decryptSearchThumbnail, createSearchTokens } from '#lib/vault/search.ts';
	import { waitUntilReady } from '#lib/vault/session.ts';

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
		matchScore?: number;
		preview?: SearchPreview;
		file?: EncryptedSearchFile;
		thumbnailUrl?: string;
	};
	type SearchAPIItem = Omit<SearchResult, 'category' | 'score' | 'matchScore' | 'preview' | 'file' | 'thumbnailUrl'>;
	type SearchResponse = {
		results?: {
			files?: EncryptedSearchFile[];
			folders?: EncryptedSearchFolder[];
			shares?: SearchAPIItem[];
		};
	};
	type Props = { open: boolean; vaultUnlocked: boolean; vaultId: string; onOpen: () => void; onClose: () => void };

	const SETTINGS = [
		{ id: 'settings-0', title: 'Instance Overview', terms: ['instance overview', 'instance', 'admin', 'email', 'storage', 'usage'], url: '/settings#settings-account', meta: 'Settings' },
		{ id: 'settings-1', title: 'Storage Provider', terms: ['storage provider', 'storage configuration', 'provider', 'local', 's3'], url: '/settings#settings-provider', meta: 'Settings' },
		{ id: 'settings-2', title: 'Security', terms: ['security', 'authentication', 'session', 'hardening'], url: '/settings#settings-security', meta: 'Settings' }
	];

	let { open, vaultUnlocked, vaultId, onOpen, onClose }: Props = $props();
	const queryClient = useQueryClient();
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

	function queryTrigrams(value: string): string[] {
		if (value.length < 3) return value ? [value] : [];
		return [...new Set(Array.from({ length: value.length - 2 }, (_, index) => value.slice(index, index + 3)))];
	}

	function rankName(query: string, name: string): number {
		const normalizedQuery = normalizeQuery(query);
		const normalizedName = normalizeQuery(name);
		if (normalizedName === normalizedQuery) return 4;
		if (normalizedName.startsWith(normalizedQuery)) return 3;
		if (normalizedName.includes(normalizedQuery)) return 2;
		const terms = queryTrigrams(normalizedQuery).slice(0, 32);
		if (!terms.length) return 0;
		const indexedTerms = new Set(queryTrigrams(normalizedName));
		return terms.filter((term) => indexedTerms.has(term)).length / terms.length;
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

	function clearSearchState() {
		if (searchTimer !== undefined) window.clearTimeout(searchTimer);
		searchTimer = undefined;
		abortController?.abort();
		abortController = undefined;

		void queryClient.cancelQueries({ queryKey: ['search', vaultId] });

		latestQuery = '';
		query = '';
		results = [];
		loading = false;
		hasSearched = false;
		activeIndex = 0;

		clearThumbnails();
	}

	function closeSearch() {
		clearSearchState();
		onClose();
	}

	async function hydrateThumbnails(items: SearchResult[], query: string, signal: AbortSignal) {
		for (const item of items) {
			const preview = item.preview;
			const file = item.file;
			const version = Number(preview?.thumbnail_version || 0);
			const size = Number(preview?.thumbnail_size || 0);
			if (!file || !preview?.has_thumbnail || version <= 0 || size <= 0 || signal.aborted) continue;

			try {
				const bytes = await getCachedThumbnail(file.id, version, size);
				if (!bytes?.length || signal.aborted) continue;
				const { thumbnailBytes } = await decryptSearchThumbnail(file, bytes);
				try {
					if (!thumbnailBytes.length || signal.aborted) continue;
					const blobBytes = Uint8Array.from(thumbnailBytes);
					let blob: Blob;
					try {
						blob = new Blob([blobBytes.buffer], { type: preview.thumbnail_mime || 'image/webp' });
					} finally {
						blobBytes.fill(0);
					}
					const url = URL.createObjectURL(blob);
					if (latestQuery !== query) {
						URL.revokeObjectURL(url);
						continue;
					}
					thumbnailURLs.add(url);
					results = results.map((result) => result.id === item.id ? { ...result, thumbnailUrl: url } : result);
				} finally {
					thumbnailBytes.fill(0);
				}
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
			await waitUntilReady();
			const tokens = await createSearchTokens(query, vaultId);
			if (controller.signal.aborted || latestQuery !== query) return;
			if (!tokens.length) {
				if (latestQuery === query) {
					results = settings;
					hasSearched = true;
				}
				return;
			}

			const data = await queryClient.fetchQuery({
				queryKey: ['search', vaultId, tokens],
				queryFn: async ({ signal }) => (await api.post<SearchResponse>(
					'/api/search',
					{ tokens, limit: 40 },
					{ signal, headers: { Accept: 'application/json' } }
				)).data,
				retry: false
			});
			if (controller.signal.aborted || latestQuery !== query) return;

			const folders = await Promise.all((data.results?.folders ?? []).map(async (folder): Promise<SearchResult | null> => {
				try {
					const opened = await decryptSearchFolder(folder);
					return {
						id: folder.id,
						kind: 'folder',
						title: opened.name?.name || opened.metadata?.name || 'Encrypted folder',
						meta: 'Folder',
						score: Number(folder.score || 0),
						matchScore: rankName(query, opened.name?.name || opened.metadata?.name || 'Encrypted folder'),
						url: `/?folder_id=${encodeURIComponent(folder.id)}`,
						category: 'results'
					};
				} catch {
					return null;
				}
			}));
			const files = await Promise.all((data.results?.files ?? []).map(async (file): Promise<SearchResult | null> => {
				try {
					const opened = await decryptSearchFile(file);
					return {
						id: file.id,
						kind: 'file',
						title: opened.metadata?.name || 'Encrypted file',
						meta: opened.metadata?.mime || 'Encrypted',
						score: Number(file.score || 0),
						matchScore: rankName(query, opened.metadata?.name || 'Encrypted file'),
						preview: opened.metadata?.preview,
						file,
						url: fileLocationHref(file.id, file.folderId),
						category: 'results'
					};
				} catch {
					return null;
				}
			}));
			if (controller.signal.aborted || latestQuery !== query) return;

			const ranked = [...folders, ...files]
				.filter((result): result is SearchResult => result !== null)
				.sort((a, b) => (b.matchScore || 0) - (a.matchScore || 0) || (b.score || 0) - (a.score || 0) || a.title.localeCompare(b.title));
			const shares = (data.results?.shares ?? []).map((result) => ({ ...result, category: 'shares' as const }));
			results = [...ranked, ...shares, ...settings];
			activeIndex = 0;
			hasSearched = true;
			void hydrateThumbnails(ranked.slice(0, 20), query, controller.signal);
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
		void queryClient.cancelQueries({ queryKey: ['search', vaultId] });
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
			closeSearch();
			void goto(result.url);
		}
	}

	function handleGlobalKeydown(event: KeyboardEvent) {
		if (open || event.key !== '/' || event.altKey || event.ctrlKey || event.metaKey) return;
		if (event.target instanceof HTMLElement && event.target.closest('input, textarea, [contenteditable="true"]')) return;
		event.preventDefault();
		onOpen();
	}

	$effect(() => {
		if (!vaultUnlocked) {
			clearSearchState();
			queryClient.removeQueries({ queryKey: ['search', vaultId] });
			if (open) onClose();
			return;
		}
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
			clearSearchState();
		};
	});
</script>

<svelte:window onkeydown={handleGlobalKeydown} />

<Modal
	{open}
	label="Search vault"
	onClose={closeSearch}
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
							onclick={closeSearch}
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
						<a id={`search-result-${index}`} href={result.url} class={`group flex items-center justify-between gap-4 rounded-md px-3.5 py-3.5 ${index === activeIndex ? 'bg-(--accent) text-(--accent-ink)' : 'text-(--text) hover:bg-(--accent) hover:text-(--accent-ink)'}`} onclick={closeSearch}>
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
						<a id={`search-result-${index}`} href={result.url} class={`group flex items-center justify-between gap-4 rounded-md px-3.5 py-3.5 ${index === activeIndex ? 'bg-(--accent) text-(--accent-ink)' : 'text-(--text) hover:bg-(--accent) hover:text-(--accent-ink)'}`} onclick={closeSearch}>
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
