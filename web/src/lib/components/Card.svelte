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

<div class={`card${className ? ` ${className}` : ''}`}>
	{#if title || subtitle}
		<div class="card-header">
			{#if title}<h3>{title}</h3>{/if}
			{#if subtitle}<p>{subtitle}</p>{/if}
		</div>
	{/if}
	{#if children}<div class="card-body">{@render children()}</div>{/if}
</div>
