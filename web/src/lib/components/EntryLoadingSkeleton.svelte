<script lang="ts">
	import Skeleton from './Skeleton.svelte';

	let {
		view,
		count = 8,
		tileSize = 180,
	}: { view: 'grid' | 'list'; count?: number; tileSize?: number } = $props();
</script>

{#if view === 'grid'}
	<div
		class="grid min-w-0"
		style={`grid-template-columns: repeat(auto-fill, minmax(${tileSize}px, 1fr)); gap: 8px; padding: 8px`}
		role="status"
		aria-label="Loading file and folder entries"
	>
		{#each Array.from({ length: count }).keys() as index (index)}
			<div class="grid min-w-0 justify-items-center gap-2 py-2">
				<Skeleton className="h-16 w-20 rounded-md" />
				<Skeleton className="h-4 w-3/4" />
			</div>
		{/each}
	</div>
{:else}
	<div class="min-w-0" role="status" aria-label="Loading file and folder entries">
		{#each Array.from({ length: count }).keys() as index (index)}
			<div
				class="flex h-11.25 items-center gap-4 border-b border-(--border-soft) px-5 last:border-0"
			>
				<div class="flex min-w-0 flex-1 items-center gap-3">
					<Skeleton className="size-5 shrink-0" />
					<Skeleton className="h-4 w-1/3" />
				</div>
				<Skeleton className="hidden h-4 w-20 sm:block" />
				<Skeleton className="h-4 w-24" />
			</div>
		{/each}
	</div>
{/if}
