<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import AppHeader from '#lib/components/AppHeader.svelte';
	import AppSidebar from '#lib/components/AppSidebar.svelte';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
	let mobileView = $state(false);
	let mobileOpen = $state(false);
	let desktopCollapsed = $state(false);

	function setMobileOpen(open: boolean) {
		mobileOpen = open;
		document.body.classList.toggle('overflow-hidden', mobileView && open);
	}

	function toggleSidebar() {
		if (mobileView) setMobileOpen(!mobileOpen);
		else desktopCollapsed = !desktopCollapsed;
	}

	function closeSidebar() {
		if (mobileView) setMobileOpen(false);
		else desktopCollapsed = true;
	}

	onMount(() => {
		const media = window.matchMedia('(max-width: 960px)');
		const syncView = () => {
			mobileView = media.matches;
			if (media.matches) desktopCollapsed = false;
			setMobileOpen(false);
		};
		const closeOnEscape = (event: KeyboardEvent) => {
			if (event.key === 'Escape') closeSidebar();
		};

		syncView();
		media.addEventListener('change', syncView);
		window.addEventListener('keydown', closeOnEscape);
		return () => {
			media.removeEventListener('change', syncView);
			window.removeEventListener('keydown', closeOnEscape);
			document.body.classList.remove('overflow-hidden');
		};
	});
</script>

<svelte:head>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="min-h-screen overflow-x-clip bg-(--bg)">
	<AppHeader
		user={data.user}
		{mobileView}
		{mobileOpen}
		{desktopCollapsed}
		onToggleSidebar={toggleSidebar}
	/>
	<AppSidebar
		user={data.user}
		currentPath={page.url.pathname}
		{mobileView}
		{mobileOpen}
		{desktopCollapsed}
		onClose={closeSidebar}
		onNavigate={() => mobileView && setMobileOpen(false)}
	/>
	<main class={`min-h-[calc(100vh-56px)] overflow-x-clip ${desktopCollapsed ? '' : 'min-[961px]:ml-60'}`} id="main-content">
		{@render children()}
	</main>
</div>
