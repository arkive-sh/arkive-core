<script lang="ts">
	import { goto } from '$app/navigation';
	import { createMutation, useQueryClient } from '@tanstack/svelte-query';
	import { Bell, CircleUser, Lock, Menu, Search } from '@lucide/svelte';
	import { logoutAndLock } from '#lib/auth.ts';
	import Dropdown from '#lib/components/Dropdown.svelte';
	import { useTransferManager } from '#lib/transfers/manager.ts';
	import { toast } from '#lib/toast.ts';

	type Props = {
		user: { name: string; username: string; email: string };
		mobileView: boolean;
		mobileOpen: boolean;
		desktopCollapsed: boolean;
		onToggleSidebar: () => void;
		onOpenSearch: () => void;
	};

	let { user, mobileView, mobileOpen, desktopCollapsed, onToggleSidebar, onOpenSearch }: Props = $props();
	const transfers = useTransferManager();
	const queryClient = useQueryClient();
	const logoutMutation = createMutation(() => ({
		mutationFn: async () => {
			await transfers.cancelUploads();
			await logoutAndLock();
		}
	}));
	let loggingOut = $derived(logoutMutation.isPending);

	async function logout() {
		try {
			await logoutMutation.mutateAsync();
			queryClient.clear();
			await goto('/login', { replace: true });
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Logout failed. Please try again.');
		}
	}
</script>

<header class={`sticky top-0 z-15 border-b border-(--border) bg-(--bg) ${desktopCollapsed ? '' : 'min-[961px]:ml-16'}`}>
	<div class="flex min-h-14 flex-wrap items-center justify-between gap-4 px-3 pt-2 pb-3 min-[721px]:px-5 min-[721px]:pb-2">
		<div class="flex min-w-0 flex-1 items-center gap-3 min-[721px]:flex-initial">
			<button
				class="inline-flex size-9 shrink-0 cursor-pointer items-center justify-center rounded border border-(--border) bg-transparent p-0 text-(--muted) hover:bg-(--surface-raised) hover:text-(--text)"
				type="button"
				aria-controls="dashboard-sidebar"
				aria-expanded={mobileView ? mobileOpen : !desktopCollapsed}
				aria-label={(mobileView ? mobileOpen : !desktopCollapsed) ? 'Close menu' : 'Open menu'}
				onclick={onToggleSidebar}
			>
				<Menu class="size-5 stroke-2" aria-hidden="true" />
			</button>
			<div class="flex min-h-10 w-full min-w-0 items-center gap-2.5 rounded border border-(--border) bg-(--surface-raised) px-3.5 min-[721px]:w-[min(456px,48vw)]">
				<Search class="size-5 shrink-0 stroke-2 text-(--muted)" aria-hidden="true" />
				<button
					class="flex min-w-0 w-full cursor-pointer items-center justify-between gap-3 border-0 bg-transparent p-0 text-left text-sm leading-5 text-(--text)"
					type="button"
					id="app-search-trigger"
					aria-label="Search vault..."
					onclick={onOpenSearch}
				>
					<span class="truncate text-(--muted)">Search vault...</span>
					<kbd class="inline-flex h-6 min-w-6 shrink-0 items-center justify-center rounded border border-(--border) bg-[rgba(48,53,57,0.85)] px-1.5 text-[0.72rem] text-(--muted)">/</kbd>
				</button>
			</div>
		</div>

		<div class="ml-auto flex items-center gap-3">
			<button class="inline-flex size-9 cursor-pointer items-center justify-center rounded bg-transparent p-0 text-(--muted) transition-colors hover:bg-(--surface-raised) hover:text-(--text)" type="button" id="app-lock-trigger" aria-label="Lock vault">
				<Lock class="size-5 stroke-2" aria-hidden="true" />
			</button>
			<a class="inline-flex size-9 items-center justify-center rounded text-(--muted) transition-colors hover:bg-(--surface-raised) hover:text-(--text)" href="/#recent-activity" aria-label="Jump to recent activity">
				<Bell class="size-5 stroke-2" aria-hidden="true" />
			</a>
			<Dropdown
				label="Open account menu"
				id="app-user-dropdown"
				rootClass="app-user-dropdown"
				triggerClass="size-9 rounded text-(--muted) transition-colors hover:bg-(--surface-raised) hover:text-(--text)"
			>
				{#snippet trigger()}
					<CircleUser class="size-5.5 stroke-2" aria-hidden="true" />
				{/snippet}
				<div class="grid gap-1 px-2 pt-1.5 pb-2">
					<span class="text-[13px] leading-4.5 font-semibold text-(--text)">{user.name || 'Account'}</span>
					<span class="text-[13px] leading-4.5 text-(--muted)">@{user.username}</span>
					<span class="break-all text-[13px] leading-4.5 text-(--muted)">{user.email}</span>
				</div>
				<div class="my-1.5 h-px bg-(--border)"></div>
				<a
					class="flex w-full items-center rounded-[10px] border border-transparent px-2.5 py-2 text-[13px] leading-4.5 text-(--text) transition-[background,border-color] duration-200 hover:border-(--border) hover:bg-(--bg-soft)"
					href="/settings"
					role="menuitem"
				>Settings</a>
				<button
					class="flex w-full cursor-pointer items-center rounded-[10px] border border-transparent px-2.5 py-2 text-left text-[13px] leading-4.5 text-(--text) transition-[background,border-color] duration-200 hover:border-(--border) hover:bg-(--bg-soft) disabled:cursor-wait disabled:opacity-60"
					type="button"
					role="menuitem"
					disabled={loggingOut}
					aria-busy={loggingOut}
					onclick={logout}
				>{loggingOut ? 'Logging out…' : 'Log out'}</button>
			</Dropdown>
		</div>
	</div>
</header>
