<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { LogOut } from '@lucide/svelte';
	import { logoutAndLock } from '#lib/auth.ts';
	import { cancelAndClearTransfers, useTransferManager } from '#lib/transfers/manager.ts';
	import { toast } from '#lib/toast.ts';
	import Tooltip from './Tooltip.svelte';

	let { side = 'right' }: { side?: 'left' | 'right' } = $props();
	let loggingOut = $state(false);
	const queryClient = useQueryClient();
	const transfers = useTransferManager();

	async function logout(): Promise<void> {
		if (loggingOut) return;
		loggingOut = true;
		queryClient.clear();
		const clearTransfers = cancelAndClearTransfers(transfers);

		try {
			try {
				await logoutAndLock();
			} finally {
				await clearTransfers;
			}
			await goto(resolve('/login'), { replace: true });
		} catch (error) {
			toast.error(error instanceof Error ? error.message : 'Logout failed. Please try again.');
		} finally {
			loggingOut = false;
		}
	}
</script>

<Tooltip label="Log out" {side}>
	<button
		class="inline-flex size-10 shrink-0 cursor-pointer items-center justify-center rounded text-(--muted) transition-colors hover:bg-(--surface-raised) hover:text-(--text) disabled:cursor-wait disabled:opacity-60"
		type="button"
		aria-label="Log out"
		aria-busy={loggingOut}
		disabled={loggingOut}
		onclick={logout}
	>
		<LogOut class="size-5 stroke-2" aria-hidden="true" />
	</button>
</Tooltip>
