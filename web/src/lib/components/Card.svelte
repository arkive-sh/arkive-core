<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		title = '',
		subtitle = '',
		class: className = '',
		children
	}: {
		title?: string;
		subtitle?: string;
		class?: string;
		children?: Snippet;
	} = $props();
</script>

<div class={`rounded-md border border-(--border) bg-(--surface) p-4 shadow-[var(--shadow)]${className ? ` ${className}` : ''}`}>
	{#if title || subtitle}
		<div class="mb-3 grid gap-1.5">
			{#if title}<h3 class="text-base font-semibold leading-6">{title}</h3>{/if}
			{#if subtitle}<p class="text-[13px] leading-4.5 text-(--muted)">{subtitle}</p>{/if}
		</div>
	{/if}
	{#if children}<div>{@render children()}</div>{/if}
</div>
