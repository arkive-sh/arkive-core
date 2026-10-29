<script lang="ts">
	import { tick } from 'svelte';
	import { FolderOpen, Info, Trash2 } from '@lucide/svelte';
	import type { FileManagerEntry } from './types';

	type Props = {
		entry: FileManagerEntry | null;
		x: number;
		y: number;
		onClose: () => void;
		onOpenFolder: (id: string) => void;
		onProperties: (entry: FileManagerEntry) => void;
		onDelete: (entry: FileManagerEntry) => void;
	};

	let { entry, x, y, onClose, onOpenFolder, onProperties, onDelete }: Props = $props();
	let menu = $state<HTMLDivElement>();
	let position = $state({ left: 8, top: 8 });

	$effect(() => {
		if (!entry) return;
		const targetX = x;
		const targetY = y;
		void tick().then(() => {
			if (!menu) return;
			const rect = menu.getBoundingClientRect();
			position = {
				left: Math.max(8, Math.min(targetX, window.innerWidth - rect.width - 8)),
				top: Math.max(8, Math.min(targetY, window.innerHeight - rect.height - 8))
			};
			menu.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus();
		});
	});

	function closeOutside(event: PointerEvent): void {
		if (entry && event.target instanceof Node && !menu?.contains(event.target)) onClose();
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (!entry) return;
		if (event.key === 'Escape') {
			event.preventDefault();
			onClose();
			return;
		}
		if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
		const items = Array.from(menu?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? []);
		if (!items.length) return;
		event.preventDefault();
		const currentIndex = items.indexOf(document.activeElement as HTMLButtonElement);
		const nextIndex = event.key === 'Home'
			? 0
			: event.key === 'End'
				? items.length - 1
				: (currentIndex + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
		items[nextIndex]?.focus();
	}

	function openFolder(): void {
		if (!entry || entry.kind !== 'folder') return;
		const id = entry.id;
		onClose();
		onOpenFolder(id);
	}

	function showProperties(): void {
		if (!entry) return;
		const selected = entry;
		onClose();
		onProperties(selected);
	}

	function confirmDelete(): void {
		if (!entry) return;
		const selected = entry;
		onClose();
		onDelete(selected);
	}
</script>

<svelte:window onpointerdown={closeOutside} onkeydown={handleKeydown} />

{#if entry}
	<div
		bind:this={menu}
		class="fixed z-50 grid w-52 gap-1 rounded-md border border-(--border) bg-(--surface) p-1.5 shadow-xl"
		style={`left: ${position.left}px; top: ${position.top}px`}
		role="menu"
		aria-label={`${entry.name} actions`}
	>
		{#if entry.kind === 'folder'}
			<button class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)" type="button" role="menuitem" onclick={openFolder}>
				<FolderOpen class="size-4 text-(--muted)" aria-hidden="true" />
				<span>Open folder</span>
			</button>
		{/if}
		<button class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm hover:bg-(--surface-highest)" type="button" role="menuitem" onclick={showProperties}>
			<Info class="size-4 text-(--muted)" aria-hidden="true" />
			<span>Properties</span>
		</button>
		<button class="inline-flex cursor-pointer items-center gap-2 rounded px-2.5 py-2 text-left text-sm text-red-400 hover:bg-red-500/10" type="button" role="menuitem" onclick={confirmDelete}>
			<Trash2 class="size-4" aria-hidden="true" />
			<span>Delete</span>
		</button>
	</div>
{/if}
