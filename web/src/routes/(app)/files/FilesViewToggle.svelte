<script lang="ts">
	import { LayoutGrid, LayoutList } from '@lucide/svelte';
	import type { FilesView } from './types';

	let { view, onChange }: { view: FilesView; onChange: (view: FilesView) => void } = $props();
</script>

<div class="inline-flex shrink-0 items-center rounded-md border border-(--border) bg-(--surface-raised) p-1" role="group" aria-label="File view">
	<button
		class={`inline-flex size-9 cursor-pointer items-center justify-center rounded transition-colors ${view === 'list' ? 'bg-(--surface-highest) text-(--text)' : 'text-(--muted) hover:text-(--text)'}`}
		type="button"
		aria-label="List view"
		aria-pressed={view === 'list'}
		onclick={() => onChange('list')}
	>
		<LayoutList class="size-4.5" aria-hidden="true" />
	</button>
	<button
		class={`inline-flex size-9 cursor-pointer items-center justify-center rounded transition-colors ${view === 'grid' ? 'bg-(--surface-highest) text-(--text)' : 'text-(--muted) hover:text-(--text)'}`}
		type="button"
		aria-label="Thumbnail view"
		aria-pressed={view === 'grid'}
		onclick={() => onChange('grid')}
	>
		<LayoutGrid class="size-4.5" aria-hidden="true" />
	</button>
</div>
