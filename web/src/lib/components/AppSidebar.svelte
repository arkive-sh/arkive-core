<script lang="ts">
	import { FolderOpen, Settings, Share2, Shield, X } from '@lucide/svelte';
	import Tooltip from '#lib/components/Tooltip.svelte';

	type Props = {
		currentPath: string;
		mobileView: boolean;
		mobileOpen: boolean;
		desktopCollapsed: boolean;
		onClose: () => void;
		onNavigate: () => void;
	};

	let { currentPath, mobileView, mobileOpen, desktopCollapsed, onClose, onNavigate }: Props = $props();
	const navigation = [
		{ href: '/', label: 'Files', icon: FolderOpen },
		{ href: '/shares', label: 'Shares', icon: Share2 },
		{ href: '/settings', label: 'Settings', icon: Settings }
	];

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
	id="app-sidebar"
	class={`fixed inset-y-0 left-0 z-20 grid w-[min(248px,86vw)] grid-rows-[auto_1fr] gap-3 border-r border-(--border) bg-(--surface) pt-4 pb-4 transition-transform duration-240 ease-[ease] translate-x-[-110%] shadow-(--shadow) ${mobileView ? 'overflow-y-auto' : 'overflow-visible'} min-[961px]:w-16 min-[961px]:shadow-none ${desktopCollapsed ? 'min-[961px]:translate-x-[-110%]' : 'min-[961px]:translate-x-0'} ${mobileView && mobileOpen ? 'max-[960px]:translate-x-0' : ''}`}
	aria-label="Primary navigation"
	aria-hidden={mobileView ? !mobileOpen : desktopCollapsed}
	inert={mobileView ? !mobileOpen : desktopCollapsed}
>
	<div class={`relative flex items-center border-b border-(--border-soft) pb-4 ${mobileView ? 'justify-between px-4' : 'justify-center px-0'}`}>
		<Tooltip label="Arkive Core" disabled={mobileView} className={mobileView ? 'flex-1' : ''}>
			<div class={`flex items-center ${mobileView ? 'gap-3' : 'justify-center'}`} role="img" aria-label="Arkive Core">
				<Shield class="size-5 shrink-0 text-(--accent)" aria-hidden="true" />
				{#if mobileView}<span class="font-mono text-lg leading-6 font-semibold">Arkive Core</span>{/if}
			</div>
		</Tooltip>
		<button class="inline-flex size-9 cursor-pointer items-center justify-center rounded border border-(--border) bg-transparent p-0 text-(--muted) hover:bg-(--surface-raised) hover:text-(--text) min-[961px]:hidden" type="button" aria-label="Close menu" onclick={onClose}>
			<X class="size-4.5 stroke-2" aria-hidden="true" />
		</button>
	</div>

	<nav class="flex flex-col gap-1 px-1 pt-1" aria-label="Main navigation">
		{#each navigation as item (item.href)}
			{@const Icon = item.icon}
			<Tooltip label={item.label} disabled={mobileView} className="w-full">
				<a class={`flex min-h-11 w-full items-center border-l-2 text-(--muted) transition-[background-color,color,border-color] duration-200 hover:bg-(--surface-raised) hover:text-(--text) focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--accent) ${mobileView ? 'gap-3 px-4 font-mono text-sm leading-5' : 'justify-center px-0'} ${isActive(item.href) ? 'border-(--accent) bg-[rgba(57,72,90,0.45)] text-(--accent)' : 'border-transparent'}`} href={item.href} aria-label={item.label} aria-current={isActive(item.href) ? 'page' : undefined} onclick={onNavigate}>
					<Icon class="size-5 shrink-0 stroke-2" aria-hidden="true" />
					{#if mobileView}<span>{item.label}</span>{/if}
				</a>
			</Tooltip>
		{/each}
	</nav>
</aside>
