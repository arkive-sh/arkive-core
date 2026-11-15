<script lang="ts">
	import { Lock, Search, Unlock } from '@lucide/svelte';
	import SidebarToggle from '#lib/components/SidebarToggle.svelte';

	type Props = {
		mobileView: boolean;
		mobileOpen: boolean;
		desktopOpen: boolean;
		sidebarTransitioning: boolean;
		vaultUnlocked: boolean;
		onToggleSidebar: () => void;
		onToggleVault: () => void;
		onOpenSearch: () => void;
	};

	let {
		mobileView,
		mobileOpen,
		desktopOpen,
		sidebarTransitioning,
		vaultUnlocked,
		onToggleSidebar,
		onToggleVault,
		onOpenSearch,
	}: Props = $props();
</script>

<header
	class={`sticky top-0 z-15 border-b border-(--border) bg-(--bg) print:hidden ${sidebarTransitioning ? 'transition-[margin-left] duration-240 ease-[ease]' : ''} ${desktopOpen ? 'min-[961px]:ml-(--sidebar-width)' : ''}`}
>
	<div
		class="flex min-h-14 flex-wrap items-center justify-between gap-4 px-3 pt-2 pb-3 min-[721px]:px-5 min-[721px]:pb-2"
	>
		<div class="flex min-w-0 flex-1 items-center gap-3 min-[721px]:flex-initial">
			<SidebarToggle
				open={mobileView ? mobileOpen : desktopOpen}
				controls="app-sidebar"
				label={mobileView ? (mobileOpen ? 'Close menu' : 'Open menu') : undefined}
				onToggle={onToggleSidebar}
			/>
			<div
				class="flex min-h-10 w-full min-w-0 items-center gap-2.5 rounded border border-(--border) bg-(--surface-raised) px-3.5 min-[721px]:w-[min(456px,48vw)]"
			>
				<Search class="size-5 shrink-0 stroke-2 text-(--muted)" aria-hidden="true" />
				<button
					class="flex min-w-0 w-full cursor-pointer items-center justify-between gap-3 border-0 bg-transparent p-0 text-left text-sm leading-5 text-(--text)"
					type="button"
					id="app-search-trigger"
					aria-label="Search vault..."
					onclick={onOpenSearch}
				>
					<span class="truncate text-(--muted)">Search vault...</span>
					<kbd
						class="inline-flex h-6 min-w-6 shrink-0 items-center justify-center rounded border border-(--border) bg-[rgba(48,53,57,0.85)] px-1.5 text-[0.72rem] text-(--muted)"
						>/</kbd
					>
				</button>
			</div>
		</div>

		<div class="ml-auto flex items-center">
			<button
				class="inline-flex size-9 cursor-pointer items-center justify-center rounded bg-transparent p-0 text-(--muted) transition-colors hover:bg-(--surface-raised) hover:text-(--text)"
				type="button"
				id="app-lock-trigger"
				aria-label={vaultUnlocked ? 'Lock vault' : 'Unlock vault'}
				title={vaultUnlocked ? 'Lock vault' : 'Unlock vault'}
				onclick={onToggleVault}
			>
				{#if vaultUnlocked}
					<Lock class="size-5 stroke-2" aria-hidden="true" />
				{:else}
					<Unlock class="size-5 stroke-2" aria-hidden="true" />
				{/if}
			</button>
		</div>
	</div>
</header>
