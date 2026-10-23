<script lang="ts">
	import { browser } from '$app/env';
	import { onMount } from 'svelte';
	import { QueryClient, QueryClientProvider } from '@tanstack/svelte-query';
	import type { Snippet } from 'svelte';
	import ToastHost from '#lib/components/ToastHost.svelte';
	import TransferCenter from '#lib/components/TransferCenter.svelte';
	import { createTransferManager, provideTransferManager } from '#lib/transfers/manager.ts';
	import '../app.css';

	let { children }: { children: Snippet } = $props();
	const queryClient = new QueryClient({
		defaultOptions: { queries: { enabled: browser, staleTime: 30_000 } }
	});
	const transferManager = createTransferManager();
	provideTransferManager(transferManager);

	onMount(() => {
		const onUploadComplete = () => {
			void queryClient.invalidateQueries({ queryKey: ['search'] });
			void queryClient.invalidateQueries({ queryKey: ['folder-entries'] });
		};
		window.addEventListener('arkive:upload-complete', onUploadComplete);
		const stored = localStorage.getItem('theme');
		const mode = ['dark', 'system', 'light'].includes(stored || '') ? stored : 'system';
		const light = window.matchMedia('(prefers-color-scheme: light)').matches;
		document.documentElement.setAttribute('data-theme', mode === 'light' || (mode === 'system' && light) ? 'light' : 'dark');
		return () => window.removeEventListener('arkive:upload-complete', onUploadComplete);
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

<QueryClientProvider client={queryClient}>
	<div class="min-h-screen bg-(--bg) font-sans text-sm leading-5 text-(--text) antialiased">
		<ToastHost />
		<TransferCenter manager={transferManager} />
		{@render children()}
	</div>
</QueryClientProvider>
