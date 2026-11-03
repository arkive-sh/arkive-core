<script lang="ts">
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { LayoutGrid, LayoutList, SlidersHorizontal } from '@lucide/svelte';
	import Tooltip from '#lib/components/Tooltip.svelte';
	import type { FilesView } from './types';

	type Props = {
		view: FilesView;
		tileSize: number;
		showType: boolean;
		showSize: boolean;
		showDimensions: boolean;
		onViewChange: (view: FilesView) => void;
		onTileSizeChange: (size: number) => void;
		onShowTypeChange: (show: boolean) => void;
		onShowSizeChange: (show: boolean) => void;
		onShowDimensionsChange: (show: boolean) => void;
	};

	let {
		view,
		tileSize,
		showType,
		showSize,
		showDimensions,
		onViewChange,
		onTileSizeChange,
		onShowTypeChange,
		onShowSizeChange,
		onShowDimensionsChange
	}: Props = $props();
	let open = $state(false);
	let root = $state<HTMLDivElement>();
	let trigger = $state<HTMLButtonElement>();
	let panel = $state<HTMLElement>();
	let position = $state<{ top: number; left: number; width: number } | null>(null);

	$effect(() => {
		if (!open) {
			position = null;
			return;
		}

		const updatePosition = () => {
			const anchor = trigger?.getBoundingClientRect();
			const panelHeight = panel?.offsetHeight ?? 0;
			if (!anchor) return;

			const margin = 8;
			const width = Math.min(288, window.innerWidth - margin * 2);
			const sidebarRight = document.getElementById('app-sidebar')?.getBoundingClientRect().right ?? 0;
			const canAvoidSidebar = sidebarRight > 0 && sidebarRight + width + margin * 2 <= window.innerWidth;
			const minLeft = canAvoidSidebar ? sidebarRight + margin : margin;
			const left = Math.max(minLeft, Math.min(anchor.right - width, window.innerWidth - width - margin));
			const below = anchor.bottom + margin;
			const top = below + panelHeight <= window.innerHeight - margin
				? below
				: Math.max(margin, anchor.top - panelHeight - margin);

			position = { top, left, width };
		};

		updatePosition();
		const resizeObserver = new ResizeObserver(updatePosition);
		if (panel) resizeObserver.observe(panel);
		window.addEventListener('resize', updatePosition);
		window.addEventListener('scroll', updatePosition, true);
		return () => {
			resizeObserver.disconnect();
			window.removeEventListener('resize', updatePosition);
			window.removeEventListener('scroll', updatePosition, true);
		};
	});

	onMount(() => {
		const closeOutside = (event: PointerEvent) => {
			if (event.target instanceof Node && !root?.contains(event.target)) open = false;
		};
		const closeOnEscape = (event: KeyboardEvent) => {
			if (event.key === 'Escape' && open) {
				open = false;
				trigger?.focus();
			}
		};
		document.addEventListener('pointerdown', closeOutside);
		window.addEventListener('keydown', closeOnEscape);
		return () => {
			document.removeEventListener('pointerdown', closeOutside);
			window.removeEventListener('keydown', closeOnEscape);
		};
	});
</script>

<div class="relative shrink-0" bind:this={root}>
	<button
		bind:this={trigger}
		class={`inline-flex size-9 cursor-pointer items-center justify-center rounded-md border p-1 transition-colors ${open ? 'border-(--border) bg-(--surface-highest) text-(--text)' : 'border-(--border) bg-(--surface-raised) text-(--muted) hover:text-(--text)'}`}
		type="button"
		aria-label="View options"
		aria-expanded={open}
		aria-controls="files-view-options"
		onclick={() => open = !open}
	>
		<SlidersHorizontal class="size-4.5" aria-hidden="true" />
	</button>
	{#if open}
		<section
			bind:this={panel}
			id="files-view-options"
			class="fixed z-50 grid gap-4 overflow-y-auto rounded-lg border border-(--border) bg-(--surface) p-4 shadow-xl"
			style={`width: ${position?.width ?? 288}px; max-height: calc(100vh - 16px); left: ${position?.left ?? -1000}px; top: ${position?.top ?? 0}px`}
			aria-label="View options"
			transition:fly={{ y: -4, duration: 140 }}
		>
			<h2 class="m-0 text-sm font-semibold">View options</h2>
			<div class="grid gap-2">
				<span class="text-xs font-medium text-(--text)">Layout</span>
				<div class="inline-flex w-fit items-center rounded-md border border-(--border) bg-(--surface-raised) p-1" role="group" aria-label="File layout">
					<Tooltip label="List view">
						<button
							class={`inline-flex size-9 cursor-pointer items-center justify-center rounded transition-colors ${view === 'list' ? 'bg-(--surface-highest) text-(--text)' : 'text-(--muted) hover:text-(--text)'}`}
							type="button"
							aria-label="List view"
							aria-pressed={view === 'list'}
							onclick={() => onViewChange('list')}
						>
							<LayoutList class="size-4.5" aria-hidden="true" />
						</button>
					</Tooltip>
					<Tooltip label="Grid view">
						<button
							class={`inline-flex size-9 cursor-pointer items-center justify-center rounded transition-colors ${view === 'grid' ? 'bg-(--surface-highest) text-(--text)' : 'text-(--muted) hover:text-(--text)'}`}
							type="button"
							aria-label="Grid view"
							aria-pressed={view === 'grid'}
							onclick={() => onViewChange('grid')}
						>
							<LayoutGrid class="size-4.5" aria-hidden="true" />
						</button>
					</Tooltip>
				</div>
			</div>
			{#if view === 'grid'}
				<label class="grid gap-2 text-xs font-medium text-(--text)">
					<span class="flex items-center justify-between gap-3">
						<span>Tile size</span>
						<output class="font-mono text-(--muted)">{tileSize}px</output>
					</span>
					<input
						class="w-full cursor-pointer accent-(--accent)"
						type="range"
						min="60"
						max="260"
						step="10"
						value={tileSize}
						aria-label="Tile size"
						aria-valuetext={`${tileSize} pixels`}
						oninput={(event) => onTileSizeChange(Number(event.currentTarget.value))}
					/>
					<span class="flex justify-between text-(--muted)"><span>Small</span><span>Large</span></span>
				</label>
				<fieldset class="m-0 grid gap-2 border-0 border-t border-(--border) p-0 pt-3">
					<legend class="mb-2 px-0 text-xs font-medium text-(--text)">Show on cards</legend>
					<label class="flex cursor-pointer items-center gap-2 text-xs text-(--muted)">
						<input class="size-4 cursor-pointer accent-(--accent)" type="checkbox" checked={showType} onchange={(event) => onShowTypeChange(event.currentTarget.checked)} />
						<span>File type</span>
					</label>
					<label class="flex cursor-pointer items-center gap-2 text-xs text-(--muted)">
						<input class="size-4 cursor-pointer accent-(--accent)" type="checkbox" checked={showSize} onchange={(event) => onShowSizeChange(event.currentTarget.checked)} />
						<span>File size</span>
					</label>
				</fieldset>
			{/if}
			<fieldset class="m-0 grid gap-2 border-0 border-t border-(--border) p-0 pt-3">
				<legend class="mb-2 px-0 text-xs font-medium text-(--text)">Show details</legend>
				<label class="flex cursor-pointer items-center gap-2 text-xs text-(--muted)">
					<input class="size-4 cursor-pointer accent-(--accent)" type="checkbox" checked={showDimensions} onchange={(event) => onShowDimensionsChange(event.currentTarget.checked)} />
					<span>Resolution</span>
				</label>
			</fieldset>
		</section>
	{/if}
</div>
