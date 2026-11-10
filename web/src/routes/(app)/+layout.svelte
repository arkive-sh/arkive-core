<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import AppHeader from '#lib/components/AppHeader.svelte';
	import AppSidebar from '#lib/components/AppSidebar.svelte';
	import SearchPalette from '#lib/components/SearchPalette.svelte';
	import { lockVault, waitUntilReady } from '#lib/vault/session.ts';
	import { vaultUnlocked } from '#lib/vault/state.ts';
	import { toast } from '#lib/toast.ts';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
	let mobileView = $state(false);
	let mobileOpen = $state(false);
	let desktopOpen = $state(true);
	let sidebarTransitioning = $state(false);
	let sidebarWidth = $state(200);
	let searchOpen = $state(false);
	let vaultReady = $state(false);
	let sidebarTransitionTimer: ReturnType<typeof setTimeout> | undefined;
	const sidebarWidthStorageKey = 'arkive.sidebar.width';

	function setMobileOpen(open: boolean) {
		mobileOpen = open;
		document.body.classList.toggle('overflow-hidden', mobileView && open);
	}

	function toggleSidebar() {
		if (mobileView) {
			setMobileOpen(!mobileOpen);

			return;
		}

		desktopOpen = !desktopOpen;
		sidebarTransitioning = true;
		clearTimeout(sidebarTransitionTimer);
		sidebarTransitionTimer = setTimeout(() => (sidebarTransitioning = false), 240);
	}

	function closeSidebar() {
		if (mobileView) setMobileOpen(false);
	}

	function openFolder(folderId: string) {
		const view = page.url.searchParams.get('view') === 'grid' ? '&view=grid' : '';
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve adds the route base before the query string.
		void goto(`${resolve('/(app)')}?folder_id=${encodeURIComponent(folderId)}${view}`);
	}

	function openFile(fileId: string, folderId: string | null) {
		const folder = folderId ? `folder_id=${encodeURIComponent(folderId)}&` : '';
		const view = page.url.searchParams.get('view') === 'grid' ? '&view=grid' : '';
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve adds the route base before the query string.
		void goto(`${resolve('/(app)')}?${folder}focus_file=${encodeURIComponent(fileId)}${view}`);
	}

	function openTrashFolder(folderId: string) {
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve adds the route base before the query string.
		void goto(`${resolve('/(app)/trash')}?folder_id=${encodeURIComponent(folderId)}`);
	}

	function openTrashFile(fileId: string, folderId: string | null) {
		const folder = folderId ? `folder_id=${encodeURIComponent(folderId)}&` : '';
		// eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve adds the route base before the query string.
		void goto(`${resolve('/(app)/trash')}?${folder}focus_file=${encodeURIComponent(fileId)}`);
	}

	function updateSidebarWidth(width: number) {
		sidebarWidth = width;
	}

	function persistSidebarWidth(width: number) {
		localStorage.setItem(sidebarWidthStorageKey, String(width));
	}

	function openSearch() {
		searchOpen = true;
	}

	function closeSearch() {
		searchOpen = false;
	}

	async function toggleVault(): Promise<void> {
		if ($vaultUnlocked) {
			try {
				await lockVault();
			} catch {
				toast.error('Could not lock the vault.');
			}
		}
		await goto(resolve('/(app)/lock'), { replace: true });
	}

	$effect(() => {
		if (vaultReady && !$vaultUnlocked && page.url.pathname !== '/lock') {
			void goto(resolve('/(app)/lock'), { replace: true });
		}
	});

	onMount(() => {
		void waitUntilReady().then(() => (vaultReady = true));
		const savedSidebarWidth = Number(localStorage.getItem(sidebarWidthStorageKey));

		if (savedSidebarWidth >= 160 && savedSidebarWidth <= 320) sidebarWidth = savedSidebarWidth;

		const media = window.matchMedia('(max-width: 960px)');
		const syncView = () => {
			mobileView = media.matches;
			setMobileOpen(false);
		};
		const closeOnEscape = (event: KeyboardEvent) => {
			if (
				event.key === 'Escape' &&
				!document.querySelector('[data-modal-open="true"], [role="menu"], .pswp--open')
			)
				closeSidebar();
		};

		syncView();
		media.addEventListener('change', syncView);
		window.addEventListener('keydown', closeOnEscape);

		return () => {
			media.removeEventListener('change', syncView);
			window.removeEventListener('keydown', closeOnEscape);
			clearTimeout(sidebarTransitionTimer);
			document.body.classList.remove('overflow-hidden');
		};
	});
</script>

<svelte:head>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="min-h-screen overflow-x-clip bg-(--bg)" style={`--sidebar-width:${sidebarWidth}px`}>
	<AppHeader
		user={data.user}
		{mobileView}
		{mobileOpen}
		{desktopOpen}
		{sidebarTransitioning}
		vaultUnlocked={$vaultUnlocked}
		onToggleSidebar={toggleSidebar}
		onToggleVault={toggleVault}
		onOpenSearch={openSearch}
	/>
	<SearchPalette
		open={searchOpen}
		vaultUnlocked={$vaultUnlocked}
		vaultId={data.user.id}
		onOpen={openSearch}
		onClose={closeSearch}
	/>
	<AppSidebar
		currentPath={page.url.pathname}
		currentFolderId={page.url.searchParams.get('folder_id')}
		currentFileId={page.url.searchParams.get('focus_file')}
		vaultId={data.user.id}
		vaultUnlocked={$vaultUnlocked}
		{mobileView}
		{mobileOpen}
		{desktopOpen}
		{sidebarWidth}
		onResize={updateSidebarWidth}
		onResizeCommit={persistSidebarWidth}
		onOpenFolder={openFolder}
		onOpenFile={openFile}
		onOpenTrashFolder={openTrashFolder}
		onOpenTrashFile={openTrashFile}
		onClose={closeSidebar}
		onNavigate={() => mobileView && setMobileOpen(false)}
	/>
	<main
		class={`min-h-[calc(100vh-56px)] overflow-x-clip ${sidebarTransitioning ? 'transition-[margin-left] duration-240 ease-[ease]' : ''} ${desktopOpen ? 'min-[961px]:ml-(--sidebar-width)' : ''}`}
		id="main-content"
	>
		{@render children()}
	</main>
</div>
