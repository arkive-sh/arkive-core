<script lang="ts">
	import { CircleUser, FolderOpen, HardDrive, LayoutDashboard, Settings, Share2, Shield, X } from '@lucide/svelte';
	import Button from '#lib/components/Button.svelte';

	type Props = {
		user: { brand_name: string };
		currentPath: string;
		mobileView: boolean;
		mobileOpen: boolean;
		desktopCollapsed: boolean;
		onClose: () => void;
		onNavigate: () => void;
	};

	let { user, currentPath, mobileView, mobileOpen, desktopCollapsed, onClose, onNavigate }: Props = $props();

	function isActive(href: string) {
		return currentPath === href || (href !== '/' && currentPath.startsWith(`${href}/`));
	}
</script>

<button
	class={`fixed inset-0 z-10 cursor-pointer bg-black/40 transition-opacity duration-200 ease-[ease] min-[961px]:hidden ${mobileView && mobileOpen ? 'pointer-events-auto opacity-100' : 'pointer-events-none opacity-0'}`}
	type="button"
	aria-hidden="true"
	tabindex="-1"
	hidden={!mobileView}
	onclick={onClose}
></button>

<aside
	id="dashboard-sidebar"
	class={`fixed inset-y-0 left-0 z-20 grid w-[min(280px,86vw)] grid-rows-[auto_auto_1fr_auto] gap-4 overflow-y-auto border-r border-(--border) bg-(--surface) pt-5 pb-4 transition-transform duration-240 ease-[ease] translate-x-[-110%] shadow-(--shadow) min-[961px]:w-60 min-[961px]:shadow-none ${desktopCollapsed ? 'min-[961px]:translate-x-[-110%]' : 'min-[961px]:translate-x-0'} ${mobileView && mobileOpen ? 'max-[960px]:translate-x-0' : ''}`}
	aria-label="Primary navigation"
	aria-hidden={mobileView ? !mobileOpen : desktopCollapsed}
	inert={mobileView ? !mobileOpen : desktopCollapsed}
>
	<div class="relative flex items-start justify-between border-b border-(--border-soft) px-4 pb-4">
		<div class="flex items-center gap-3">
			<span class="inline-flex size-12 items-center justify-center rounded-full border border-(--border) bg-(--surface-raised) text-(--text)">
				<CircleUser class="size-5.5 stroke-2" aria-hidden="true" />
			</span>
			<div class="grid gap-1">
				<a class="font-mono text-lg leading-6 font-semibold" href="/" onclick={onNavigate}>Arkive Core</a>
				<span class="font-mono text-[13px] leading-4.5 text-(--muted)">{user.brand_name ? `Node: ${user.brand_name}` : 'Self-hosted core'}</span>
			</div>
		</div>
		<button class="absolute top-0 right-4 inline-flex size-9 cursor-pointer items-center justify-center rounded border border-(--border) bg-transparent p-0 text-(--muted) hover:bg-(--surface-raised) hover:text-(--text) min-[961px]:hidden" type="button" aria-label="Close menu" onclick={onClose}>
			<X class="size-4.5 stroke-2" aria-hidden="true" />
		</button>
	</div>

	<div class="border-b border-(--border-soft) px-4 pb-4">
		<Button text="Upload File" variant="primary" href="/#upload-panel" class="w-full" />
	</div>

	<nav class="flex flex-col gap-0.5 pt-1" aria-label="Main navigation">
		<a class={`flex min-h-11 items-center gap-3 border-l-2 px-4 font-mono text-sm leading-5 text-(--muted) transition-[background-color,color,border-color] duration-200 hover:bg-(--surface-raised) hover:text-(--text) ${isActive('/') ? 'border-(--accent) bg-[rgba(57,72,90,0.45)] pl-3.5 text-(--accent)' : 'border-transparent'}`} href="/" aria-current={isActive('/') ? 'page' : undefined} onclick={onNavigate}>
			<LayoutDashboard class="size-4.5 shrink-0 stroke-2" aria-hidden="true" />
			<span>Dashboard</span>
		</a>
		<a class={`flex min-h-11 items-center gap-3 border-l-2 px-4 font-mono text-sm leading-5 text-(--muted) transition-[background-color,color,border-color] duration-200 hover:bg-(--surface-raised) hover:text-(--text) ${isActive('/files') ? 'border-(--accent) bg-[rgba(57,72,90,0.45)] pl-3.5 text-(--accent)' : 'border-transparent'}`} href="/files" aria-current={isActive('/files') ? 'page' : undefined} onclick={onNavigate}>
			<FolderOpen class="size-4.5 shrink-0 stroke-2" aria-hidden="true" />
			<span>Files</span>
		</a>
		<a class={`flex min-h-11 items-center gap-3 border-l-2 px-4 font-mono text-sm leading-5 text-(--muted) transition-[background-color,color,border-color] duration-200 hover:bg-(--surface-raised) hover:text-(--text) ${isActive('/shares') ? 'border-(--accent) bg-[rgba(57,72,90,0.45)] pl-3.5 text-(--accent)' : 'border-transparent'}`} href="/shares" aria-current={isActive('/shares') ? 'page' : undefined} onclick={onNavigate}>
			<Share2 class="size-4.5 shrink-0 stroke-2" aria-hidden="true" />
			<span>Shares</span>
		</a>
		<a class={`flex min-h-11 items-center gap-3 border-l-2 px-4 font-mono text-sm leading-5 text-(--muted) transition-[background-color,color,border-color] duration-200 hover:bg-(--surface-raised) hover:text-(--text) ${isActive('/settings') ? 'border-(--accent) bg-[rgba(57,72,90,0.45)] pl-3.5 text-(--accent)' : 'border-transparent'}`} href="/settings" aria-current={isActive('/settings') ? 'page' : undefined} onclick={onNavigate}>
			<Settings class="size-4.5 shrink-0 stroke-2" aria-hidden="true" />
			<span>Settings</span>
		</a>
	</nav>

	<div class="grid gap-0.5 border-t border-(--border-soft) pt-4">
		<a class="flex min-h-11 items-center gap-3 px-4 font-mono text-sm leading-5 text-(--muted) transition-[background-color,color,border-color] duration-200 hover:bg-(--surface-raised) hover:text-(--text)" href="/settings" onclick={onNavigate}>
			<HardDrive class="size-4.5 shrink-0 stroke-2" aria-hidden="true" />
			<span>Storage Status</span>
		</a>
		<div class="flex min-h-11 items-center gap-3 px-4 font-mono text-sm leading-5 text-(--muted)">
			<Shield class="size-4.5 shrink-0 stroke-2" aria-hidden="true" />
			<span>Single-user core</span>
		</div>
	</div>
</aside>
