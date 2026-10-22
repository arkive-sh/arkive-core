<script lang="ts">
	import { onMount } from 'svelte';
	import type { Snippet } from 'svelte';
	import ToastHost from '#lib/components/ToastHost.svelte';
	import '../app.css';

	let { children }: { children: Snippet } = $props();
	onMount(() => {
		const stored = localStorage.getItem('theme');
		const mode = ['dark', 'system', 'light'].includes(stored || '') ? stored : 'system';
		const light = window.matchMedia('(prefers-color-scheme: light)').matches;
		document.documentElement.setAttribute('data-theme', mode === 'light' || (mode === 'system' && light) ? 'light' : 'dark');
	});
</script>

<svelte:head>
	<link rel="preconnect" href="https://fonts.googleapis.com" />
	<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin="anonymous" />
	<link
		rel="stylesheet"
		href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600&display=swap"
	/>
	<link rel="icon" type="image/x-icon" href="/static/assets/images/favicon.ico" />
	<link rel="manifest" href="/static/assets/images/site.webmanifest" />
</svelte:head>

<div class="min-h-screen bg-(--bg) font-sans text-sm leading-5 text-(--text) antialiased">
	<ToastHost />
	{@render children()}
</div>
