<script lang="ts">
	import { createInfiniteQuery } from '@tanstack/svelte-query';
	import { ChevronRight, Folder } from '@lucide/svelte';
	import FileTypeIcon from './FileTypeIcon.svelte';
	import InfiniteScrollSentinel from './InfiniteScrollSentinel.svelte';
	import FolderTreeNode from './FolderTreeNode.svelte';
	import type { FileManagerEntry } from '../../routes/(app)/files/types';
	import type { FolderTreeSource } from './folder-tree.svelte';

	type Props = {
		folder: FileManagerEntry | null;
		rootFolderId: string | null;
		depth: number;
		path: FileManagerEntry[];
		source: FolderTreeSource;
		enabled: boolean;
		currentFolderId: string | null;
		currentFileId: string | null;
		onOpenFolder: (folderId: string, path: FileManagerEntry[]) => void;
		onOpenFile: (entry: FileManagerEntry, path: FileManagerEntry[]) => void;
	};

	let {
		folder,
		rootFolderId,
		depth,
		path,
		source,
		enabled,
		currentFolderId,
		currentFileId,
		onOpenFolder,
		onOpenFile,
	}: Props = $props();
	let expanded = $state(false);
	let children = $state<FileManagerEntry[]>([]);
	let decryptGeneration = 0;
	const parentFolderId = $derived(folder?.id ?? rootFolderId);
	const branchExpanded = $derived(folder === null || expanded);

	const query = createInfiniteQuery(() => ({
		queryKey: source.queryKey(parentFolderId),
		enabled: enabled && branchExpanded,
		initialPageParam: null as string | null,
		queryFn: async ({ pageParam, signal }) => source.loadPage(parentFolderId, pageParam, signal),
		getNextPageParam: (lastPage) =>
			lastPage.hasMore ? lastPage.nextCursor || undefined : undefined,
	}));
	const lastPage = $derived(query.data?.pages.at(-1));

	$effect(() => {
		const pages = query.data?.pages ?? [];
		const generation = ++decryptGeneration;

		if (!enabled || !branchExpanded) {
			children = [];

			return;
		}

		void Promise.all(pages.map((page) => source.decryptPage(page.payload, parentFolderId))).then(
			(decryptedPages) => {
				if (generation === decryptGeneration && enabled) children = decryptedPages.flat();
			},
		);
	});

	function openFolder(entry: FileManagerEntry): void {
		expanded = true;
		onOpenFolder(entry.id, [...path, entry]);
	}
</script>

{#if folder}
	<div
		class={`flex min-h-8 items-center gap-1 rounded pr-1 text-sm leading-5 ${folder.id === currentFolderId ? 'bg-(--surface-raised) text-(--text)' : 'text-(--muted) hover:bg-(--surface-raised) hover:text-(--text)'}`}
		style:padding-left={`${depth * 16}px`}
	>
		<button
			class="inline-flex size-7 shrink-0 cursor-pointer items-center justify-center rounded hover:bg-(--surface-highest) focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-(--accent)"
			type="button"
			aria-label={`${expanded ? 'Collapse' : 'Expand'} ${folder.name}`}
			aria-expanded={expanded}
			onclick={() => (expanded = !expanded)}
		>
			<ChevronRight
				class={`size-4 transition-transform ${expanded ? 'rotate-90' : ''}`}
				aria-hidden="true"
			/>
		</button>
		<button
			class="flex min-w-0 flex-1 cursor-pointer items-center gap-2 truncate text-left focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-(--accent)"
			type="button"
			aria-current={folder.id === currentFolderId ? 'location' : undefined}
			title={folder.name}
			onclick={() => openFolder(folder)}
		>
			<Folder class="size-4 shrink-0 text-(--accent)" aria-hidden="true" />
			<span class="truncate">{folder.name}</span>
		</button>
	</div>
{/if}

{#if branchExpanded}
	<div class="grid gap-0.5">
		{#if query.isPending}
			<div class="py-1 text-xs text-(--muted)" style:padding-left={`${depth * 16 + 28}px`}>
				Loading items…
			</div>
		{:else if query.isError}
			<button
				class="py-1 text-left text-xs text-(--muted) hover:text-(--text)"
				style:padding-left={`${depth * 16 + 28}px`}
				type="button"
				onclick={() => void query.refetch()}>Could not load items. Retry</button
			>
		{:else if !children.length}
			<div class="py-1 text-xs text-(--muted)" style:padding-left={`${depth * 16 + 28}px`}>
				No items
			</div>
		{:else}
			{#each children as child (child.kind + ':' + child.id)}
				{#if child.kind === 'folder'}
					<FolderTreeNode
						folder={child}
						rootFolderId={null}
						depth={depth + 1}
						path={[...path, ...(folder ? [folder] : [])]}
						{source}
						{enabled}
						{currentFolderId}
						{currentFileId}
						{onOpenFolder}
						{onOpenFile}
					/>
				{:else}
					<div
						class={`flex min-h-8 items-center rounded pr-1 text-sm leading-5 ${child.id === currentFileId ? 'bg-(--surface-raised) text-(--text)' : 'text-(--muted) hover:bg-(--surface-raised) hover:text-(--text)'}`}
						style:padding-left={`${depth * 16 + 28}px`}
					>
						<button
							class="flex min-w-0 flex-1 cursor-pointer items-center gap-2 truncate text-left focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-(--accent)"
							type="button"
							aria-current={child.id === currentFileId ? 'location' : undefined}
							title={child.name}
							onclick={() => onOpenFile(child, [...path, ...(folder ? [folder] : [])])}
						>
							<FileTypeIcon name={child.name} mime={child.type} className="size-4 shrink-0" />
							<span class="truncate">{child.name}</span>
						</button>
					</div>
				{/if}
			{/each}
		{/if}
		{#if query.hasNextPage && lastPage}
			{#if query.isFetchNextPageError}
				<button
					class="py-1 text-left text-xs text-red-400 hover:text-(--text)"
					style:padding-left={`${depth * 16 + 28}px`}
					type="button"
					onclick={() => void query.fetchNextPage()}>Could not load more. Retry</button
				>
			{:else if query.isFetchingNextPage}
				<div
					class="py-1 text-xs text-(--muted)"
					style:padding-left={`${depth * 16 + 28}px`}
					role="status"
				>
					Loading more…
				</div>
			{:else}
				<InfiniteScrollSentinel
					enabled={query.hasNextPage && !query.isFetching}
					onLoad={() => void query.fetchNextPage()}
					rootSelector="[data-folder-tree-scroll-root]"
					rootMargin="200px"
				/>
			{/if}
		{/if}
	</div>
{/if}
