<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		number,
		title,
		icon,
		children
	}: { number: string; title: string; icon: Snippet; children: Snippet } = $props();
</script>

<section class="grid gap-4 border-b border-(--border-soft) py-5 last:border-0">
	<header class="flex items-center gap-2">
		<span class="inline-flex items-center justify-center text-(--accent-strong)">{@render icon()}</span>
		<h2 class="m-0 text-base leading-6 font-semibold">{number}. {title}</h2>
	</header>
	<div class="grid gap-4">{@render children()}</div>
</section>
