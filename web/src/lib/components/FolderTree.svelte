<script lang="ts">
	import { ChevronRight, Folder } from '@lucide/svelte';
	import FolderTreeNode from './FolderTreeNode.svelte';
	import type { FileManagerEntry } from '../../routes/(app)/files/types';
	import type { FolderTreeSource } from './folder-tree.svelte';

	type Props = {
		source: FolderTreeSource;
		enabled: boolean;
		rootFolderId: string | null;
		rootName: string;
		showRoot?: boolean;
		open?: boolean;
		currentFolderId: string | null;
		currentFileId: string | null;
		onOpenFolder: (folderId: string, path: FileManagerEntry[]) => void;
		onOpenFile: (entry: FileManagerEntry, path: FileManagerEntry[]) => void;
	};

	let {
		source,
		enabled,
		rootFolderId,
		rootName,
		showRoot = false,
		open = true,
		currentFolderId,
		currentFileId,
		onOpenFolder,
		onOpenFile,
	}: Props = $props();
	let rootExpanded = $state(true);
</script>

{#if showRoot}
	<div
		class={`flex min-h-8 items-center gap-1 rounded pr-1 text-sm leading-5 ${rootFolderId === currentFolderId ? 'bg-(--surface-raised) text-(--text)' : 'text-(--muted) hover:bg-(--surface-raised) hover:text-(--text)'}`}
	>
		<button
			class="inline-flex size-7 shrink-0 cursor-pointer items-center justify-center rounded hover:bg-(--surface-highest) focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-(--accent)"
			type="button"
			aria-label={`${rootExpanded ? 'Collapse' : 'Expand'} ${rootName}`}
			aria-expanded={rootExpanded}
			onclick={() => (rootExpanded = !rootExpanded)}
		>
			<ChevronRight
				class={`size-4 transition-transform ${rootExpanded ? 'rotate-90' : ''}`}
				aria-hidden="true"
			/>
		</button>
		<button
			class="flex min-w-0 flex-1 cursor-pointer items-center gap-2 truncate text-left focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-(--accent)"
			type="button"
			aria-current={rootFolderId === currentFolderId ? 'location' : undefined}
			title={rootName}
			onclick={() => {
				rootExpanded = true;
				if (rootFolderId) onOpenFolder(rootFolderId, []);
			}}
		>
			<Folder class="size-4 shrink-0 text-(--accent)" aria-hidden="true" />
			<span class="truncate">{rootName}</span>
		</button>
	</div>
{/if}

{#if enabled && (showRoot ? rootExpanded : open)}
	<FolderTreeNode
		folder={null}
		{rootFolderId}
		depth={showRoot ? 1 : 0}
		path={[]}
		{source}
		{enabled}
		{currentFolderId}
		{currentFileId}
		{onOpenFolder}
		{onOpenFile}
	/>
{/if}
