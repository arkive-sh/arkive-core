<script lang="ts">
	import { Menu } from '@lucide/svelte';

	let {
		open,
		controls,
		label,
		onToggle,
	}: {
		open: boolean;
		controls: string;
		label?: string;
		onToggle: () => void;
	} = $props();
</script>

<button
	class="inline-flex size-9 shrink-0 cursor-pointer items-center justify-center rounded border border-(--border) bg-transparent p-0 text-(--muted) hover:bg-(--surface-raised) hover:text-(--text)"
	type="button"
	aria-controls={controls}
	aria-expanded={open}
	aria-label={label ?? (open ? 'Hide sidebar' : 'Show sidebar')}
	title={label ?? (open ? 'Hide sidebar' : 'Show sidebar')}
	onclick={onToggle}
>
	<Menu class="size-5 stroke-2" aria-hidden="true" />
</button>
