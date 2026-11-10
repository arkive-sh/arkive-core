<script lang="ts">
	import { resolve } from '$app/paths';
	import { ChevronRight, FolderOpen, Settings, Share2, Trash2, X } from '@lucide/svelte';
	import FolderTree from './FolderTree.svelte';
	import SidebarResizeHandle from './SidebarResizeHandle.svelte';
	import { createFilesFolderTreeSource, createTrashFolderTreeSource } from './folder-tree.svelte';
	import type { FileManagerEntry } from '../../routes/(app)/files/types';

	type Props = {
		currentPath: string;
		currentFolderId: string | null;
		currentFileId: string | null;
		vaultId: string;
		vaultUnlocked: boolean;
		mobileView: boolean;
		mobileOpen: boolean;
		desktopOpen: boolean;
		sidebarWidth: number;
		onResize: (width: number) => void;
		onResizeCommit: (width: number) => void;
		onOpenFolder: (folderId: string) => void;
		onOpenFile: (fileId: string, folderId: string | null) => void;
		onOpenTrashFolder: (folderId: string) => void;
		onOpenTrashFile: (fileId: string, folderId: string | null) => void;
		onClose: () => void;
		onNavigate: () => void;
	};

	let {
		currentPath,
		currentFolderId,
		currentFileId,
		vaultId,
		vaultUnlocked,
		mobileView,
		mobileOpen,
		desktopOpen,
		sidebarWidth,
		onResize,
		onResizeCommit,
		onOpenFolder,
		onOpenFile,
		onOpenTrashFolder,
		onOpenTrashFile,
		onClose,
		onNavigate,
	}: Props = $props();
	let folderTreeOpen = $state(true);
	let trashTreeOpen = $state(true);
	const navigation = [{ route: '/(app)/shares', label: 'Shares', icon: Share2 }] as const;
	const filesHref = resolve('/(app)');
	const trashHref = resolve('/(app)/trash');
	const trashActive = $derived(isActive(trashHref));
	const filesTreeSource = createFilesFolderTreeSource(() => vaultId);
	const trashTreeSource = createTrashFolderTreeSource(() => vaultId);

	function isActive(path: string) {
		return currentPath === path || (path !== '/' && currentPath.startsWith(`${path}/`));
	}

	function openFolder(folderId: string) {
		onOpenFolder(folderId);
		onNavigate();
	}

	function openFile(fileId: string, folderId: string | null) {
		onOpenFile(fileId, folderId);
		onNavigate();
	}

	function openTrashFolder(folderId: string) {
		onOpenTrashFolder(folderId);
		onNavigate();
	}

	function openTrashFile(entry: FileManagerEntry) {
		onOpenTrashFile(entry.id, entry.parentFolderId ?? null);
		onNavigate();
	}
</script>

<button
	class={`fixed inset-0 z-10 cursor-pointer bg-black/40 transition-opacity duration-200 ease-[ease] min-[961px]:hidden ${mobileView && mobileOpen ? 'pointer-events-auto opacity-100' : 'pointer-events-none opacity-0'}`}
	type="button"
	aria-hidden="true"
	tabindex="-1"
	hidden={!mobileView}
	onclick={onClose}
></button>

<aside
	id="app-sidebar"
	data-folder-tree-scroll-root
	class={`fixed inset-y-0 left-0 z-20 grid w-[min(248px,86vw)] grid-rows-[auto_1fr_auto] overflow-y-auto border-r border-(--border) pt-4 pb-4 transition-transform duration-240 ease-[ease] translate-x-[-110%] shadow-(--shadow) ${mobileView && mobileOpen ? 'max-[960px]:translate-x-0' : ''} min-[961px]:w-(--sidebar-width) ${desktopOpen ? 'min-[961px]:translate-x-0' : 'min-[961px]:translate-x-[-110%]'} min-[961px]:shadow-none`}
	aria-label="Primary navigation"
	aria-hidden={(mobileView && !mobileOpen) || (!mobileView && !desktopOpen)}
	inert={(mobileView && !mobileOpen) || (!mobileView && !desktopOpen)}
>
	<div class="flex items-center justify-between border-b border-(--border-soft) px-4 pb-4">
		<h2 class="text-base leading-6 font-semibold text-(--text)">Arkive</h2>
		<button
			class="inline-flex size-9 cursor-pointer items-center justify-center rounded border border-(--border) bg-transparent p-0 text-(--muted) hover:bg-(--surface-raised) hover:text-(--text) min-[961px]:hidden"
			type="button"
			aria-label="Close menu"
			onclick={onClose}
		>
			<X class="size-4.5 stroke-2" aria-hidden="true" />
		</button>
	</div>

	<nav class="flex flex-col gap-1 px-3 pt-3" aria-label="Main navigation">
		<div>
			<div class="flex min-w-0 items-center gap-1">
				<button
					class="inline-flex size-8 shrink-0 cursor-pointer items-center justify-center rounded text-(--muted) hover:bg-(--surface-raised) hover:text-(--text) focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-(--accent)"
					type="button"
					aria-label={`${folderTreeOpen ? 'Collapse' : 'Expand'} folder tree`}
					aria-expanded={folderTreeOpen}
					onclick={() => (folderTreeOpen = !folderTreeOpen)}
				>
					<ChevronRight
						class={`size-4 transition-transform ${folderTreeOpen ? 'rotate-90' : ''}`}
						aria-hidden="true"
					/>
				</button>
				<a
					class={`flex min-h-10 min-w-0 flex-1 items-center gap-3 rounded px-3 text-sm leading-5 transition-colors hover:bg-(--surface-raised) ${isActive(filesHref) ? 'bg-(--accent)/15 text-(--accent)' : 'text-(--muted) hover:text-(--text)'}`}
					href={filesHref}
					aria-current={isActive(filesHref) ? 'page' : undefined}
					onclick={onNavigate}
				>
					<FolderOpen class="size-5 shrink-0 stroke-2" aria-hidden="true" />
					<span>Files</span>
				</a>
			</div>
			{#if vaultUnlocked}
				<div role="group" aria-label="Folder tree" class="mt-1">
					<FolderTree
						source={filesTreeSource}
						enabled={vaultUnlocked}
						rootFolderId={null}
						rootName="Files"
						open={folderTreeOpen}
						{currentFolderId}
						{currentFileId}
						onOpenFolder={(folderId) => openFolder(folderId)}
						onOpenFile={(entry) => openFile(entry.id, entry.parentFolderId ?? null)}
					/>
				</div>
			{/if}
		</div>
		<div>
			<div class="flex min-w-0 items-center gap-1">
				<button
					class="inline-flex size-8 shrink-0 cursor-pointer items-center justify-center rounded text-(--muted) hover:bg-(--surface-raised) hover:text-(--text) focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-(--accent)"
					type="button"
					aria-label={`${trashTreeOpen ? 'Collapse' : 'Expand'} Trash tree`}
					aria-expanded={trashTreeOpen}
					onclick={() => (trashTreeOpen = !trashTreeOpen)}
				>
					<ChevronRight
						class={`size-4 transition-transform ${trashTreeOpen ? 'rotate-90' : ''}`}
						aria-hidden="true"
					/>
				</button>
				<a
					class={`flex min-h-10 min-w-0 flex-1 items-center gap-3 rounded px-3 text-sm leading-5 transition-colors hover:bg-(--surface-raised) ${trashActive ? 'bg-(--accent)/15 text-(--accent)' : 'text-(--muted) hover:text-(--text)'}`}
					href={trashHref}
					aria-current={trashActive ? 'page' : undefined}
					onclick={onNavigate}
				>
					<Trash2 class="size-5 shrink-0 stroke-2" aria-hidden="true" />
					<span>Trash</span>
				</a>
			</div>
			{#if trashActive && trashTreeOpen && vaultUnlocked}
				<div role="group" aria-label="Trash folder tree" class="mt-1">
					<FolderTree
						source={trashTreeSource}
						enabled={vaultUnlocked}
						rootFolderId={null}
						rootName="Trash"
						{currentFolderId}
						{currentFileId}
						onOpenFolder={(folderId) => openTrashFolder(folderId)}
						onOpenFile={openTrashFile}
					/>
				</div>
			{/if}
		</div>
		{#each navigation as item (item.route)}
			{@const Icon = item.icon}
			{@const href = resolve(item.route)}
			<a
				class={`flex min-h-10 items-center gap-3 rounded px-3 text-sm leading-5 text-(--muted) transition-colors hover:bg-(--surface-raised) hover:text-(--text) focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--accent) ${isActive(href) ? 'bg-(--accent)/15 text-(--accent)' : ''}`}
				{href}
				aria-current={isActive(href) ? 'page' : undefined}
				onclick={onNavigate}
			>
				<Icon class="size-5 shrink-0 stroke-2" aria-hidden="true" />
				<span>{item.label}</span>
			</a>
		{/each}
	</nav>
	<div class="border-t border-(--border-soft) px-3 pt-3">
		<!-- eslint-disable svelte/no-navigation-without-resolve -->
		<a
			class="flex min-h-10 items-center gap-3 rounded px-3 text-sm leading-5 text-(--muted) transition-colors hover:bg-(--surface-raised) hover:text-(--text) focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--accent)"
			href="/settings"
			onclick={onNavigate}
		>
			<Settings class="size-5 shrink-0 stroke-2" aria-hidden="true" />
			<span>Settings</span>
		</a>
		<!-- eslint-enable svelte/no-navigation-without-resolve -->
	</div>
	<SidebarResizeHandle width={sidebarWidth} visible={!mobileView} {onResize} {onResizeCommit} />
</aside>
