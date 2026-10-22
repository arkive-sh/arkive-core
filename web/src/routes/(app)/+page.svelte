<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { File, Lock } from '@lucide/svelte';
	import { apiRequest } from '#lib/api.ts';
	import type { DashboardRecentFile, DashboardResponse } from './dashboard';
	import FileUploader from '#lib/components/FileUploader.svelte';
	import { decryptSearchFile } from '#lib/vault/search.ts';
	import { waitUntilReady } from '#lib/vault/session.ts';
	import { vaultUnlocked } from '#lib/vault/state.ts';
	import type { PageProps } from './$types';

	type RecentFile = DashboardRecentFile & { name: string; size?: number; hydrated: boolean };
	let { data }: PageProps = $props();
	let dashboard = $state<DashboardResponse>(untrack(() => data.dashboard));
	let recentFiles = $state<RecentFile[]>(untrack(() => dashboard.recentFiles.map((file) => ({ ...file, name: '', hydrated: false }))));
	let dashboardError = $state('');
	let vaultError = $state('');
	let hydration = 0;

	const usedPercent = $derived(
		dashboard.maxStorageBytes > 0
			? Math.min((dashboard.usedBytes / dashboard.maxStorageBytes) * 100, 100)
			: 0
	);
	const percentText = $derived(usedPercent > 0 ? `${usedPercent.toFixed(1)}%` : '0%');

	$effect(() => {
		if ($vaultUnlocked) return;
		recentFiles = dashboard.recentFiles.map((file) => ({ ...file, name: '', hydrated: false }));
		vaultError = dashboard.recentFiles.length ? 'Unlock your vault to view file names and sizes.' : '';
	});

	function formatBytes(bytes: number): string {
		if (bytes <= 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB'];
		const unit = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
		return `${(bytes / 1024 ** unit).toFixed(unit === 0 ? 0 : unit === 3 ? 2 : 1)} ${units[unit]}`;
	}

	function relativeTime(value: string): string {
		const date = new Date(value);
		const difference = Date.now() - date.getTime();
		if (!Number.isFinite(difference)) return '';
		if (difference < 0) {
			return new Intl.DateTimeFormat('en-US', {
				month: 'short', day: 'numeric', year: 'numeric', hour: '2-digit', minute: '2-digit', hourCycle: 'h23'
			}).format(date);
		}
		if (difference < 60_000) return 'Just now';
		if (difference < 3_600_000) return `${Math.floor(difference / 60_000)}m ago`;
		if (difference < 86_400_000) return `${Math.floor(difference / 3_600_000)}h ago`;
		if (difference < 604_800_000) return `${Math.floor(difference / 86_400_000)}d ago`;
		if (difference < 2_592_000_000) return `${Math.floor(difference / 604_800_000)}w ago`;
		return new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric', year: 'numeric' }).format(date);
	}

	async function hydrateRecentFiles(): Promise<void> {
		const currentHydration = ++hydration;
		const files = dashboard.recentFiles;
		recentFiles = files.map((file) => ({ ...file, name: '', hydrated: false }));
		vaultError = '';

		if (!(await waitUntilReady()).unlocked) {
			vaultError = 'Unlock your vault to view file names and sizes.';
			return;
		}

		const opened = await Promise.all(files.map(async (file): Promise<RecentFile> => {
			try {
				const { metadata } = await decryptSearchFile({ ...file, score: 0, url: `/files/${file.id}/view` });
				return {
					...file,
					name: typeof metadata.name === 'string' ? metadata.name : 'Encrypted file',
					size: typeof metadata.size === 'number' ? metadata.size : undefined,
					hydrated: true
				};
			} catch {
				return { ...file, name: 'Encrypted file', hydrated: true };
			}
		}));

		if (currentHydration === hydration && $vaultUnlocked) recentFiles = opened;
	}

	async function refreshDashboard(): Promise<void> {
		try {
			dashboard = await apiRequest<DashboardResponse>('/api/dashboard', {}, 'Could not refresh dashboard');
			dashboardError = '';
			await hydrateRecentFiles();
		} catch (error) {
			dashboardError = error instanceof Error ? error.message : 'Could not refresh dashboard';
		}
	}

	onMount(() => {
		void hydrateRecentFiles();
	});
</script>

<svelte:head>
	<title>Arkive · Vault</title>
</svelte:head>

<div class="min-h-full overflow-x-clip bg-(--bg) py-6 pb-8">
	<div class="mx-auto grid w-[min(1440px,calc(100%-40px))] gap-6 max-[960px]:w-[calc(100%-24px)]">
		<section class="grid grid-cols-2 gap-3 max-[960px]:grid-cols-1" aria-label="Vault storage summary">
			<article class="grid gap-3 rounded-md border border-(--border) bg-(--surface-raised) px-6 py-5">
				<h2 class="m-0 text-base leading-6 font-semibold tracking-[0.02em] text-(--muted)">Stored Files</h2>
				<p class="m-0 text-[2rem] leading-[1.05] font-semibold text-(--accent-strong)">{dashboard.totalFiles.toLocaleString('en-US')}</p>
			</article>

			<article class="grid gap-3 rounded-md border border-(--border) bg-(--surface-raised) px-6 py-5">
				<div class="flex items-baseline justify-between gap-3">
					<h2 class="m-0 text-base leading-6 font-semibold tracking-[0.02em] text-(--muted)">Storage Used</h2>
					<span class="text-sm leading-5 text-(--muted)">{percentText}</span>
				</div>
				<div class="flex items-baseline justify-between gap-3">
					<p class="m-0 text-[2rem] leading-[1.05] font-semibold text-(--accent-strong)">{formatBytes(dashboard.usedBytes)}</p>
					<span class="text-sm leading-5 text-(--muted)">
						{dashboard.maxStorageBytes > 0 ? `/ ${formatBytes(dashboard.maxStorageBytes)}` : '/ Unlimited'}
					</span>
				</div>
				{#if dashboard.maxStorageBytes > 0}
					<div class="mt-1 h-2 overflow-hidden rounded-full bg-(--surface-highest)" role="progressbar" aria-label="Storage used" aria-valuemin="0" aria-valuemax="100" aria-valuenow={usedPercent}>
						<span class="block h-full bg-(--accent)" style={`width: ${usedPercent}%`}></span>
					</div>
				{/if}
			</article>
		</section>

		<FileUploader onUploaded={refreshDashboard} />

		<section class="grid gap-3" id="recent-activity" aria-labelledby="recent-files-heading">
			<h2 id="recent-files-heading" class="m-0 text-lg leading-6 font-semibold tracking-[0.02em] text-(--muted)">Recent Files</h2>
			{#if dashboardError}
				<p class="m-0 text-sm text-(--danger)" role="alert">{dashboardError}</p>
			{/if}
			{#if vaultError}
				<p class="m-0 text-sm text-(--muted)" role="status">{vaultError}</p>
			{/if}
			<div class="overflow-auto rounded-md border border-(--border) bg-(--surface-raised)">
				<table class="w-full min-w-[760px] table-fixed border-collapse text-left">
					<thead class="border-b border-(--border) bg-(--surface-highest)">
						<tr>
							<th class="px-4 py-3 text-[11px] leading-4 font-semibold tracking-wider text-(--muted) uppercase">File Name</th>
							<th class="w-[140px] px-4 py-3 text-[11px] leading-4 font-semibold tracking-wider text-(--muted) uppercase">Size</th>
							<th class="w-[180px] px-4 py-3 text-[11px] leading-4 font-semibold tracking-wider text-(--muted) uppercase">Modified</th>
						</tr>
					</thead>
					<tbody>
						{#if recentFiles.length === 0}
							<tr>
								<td class="px-4 py-7 text-(--muted)" colspan="3">No completed files yet.</td>
							</tr>
						{:else}
							{#each recentFiles as file (file.id)}
								<tr class="border-b border-(--border) last:border-0 hover:bg-(--surface)">
									<td class="px-4 py-3">
										<div class="flex min-w-0 items-center gap-2.5">
											<File class="size-4 shrink-0 text-(--muted-strong)" aria-hidden="true" />
											<span class="min-w-0 truncate text-(--text)" aria-busy={!file.hydrated}>
												{#if file.hydrated}{file.name}{:else}<span class="inline-block h-[1.05rem] w-[min(240px,60vw)] animate-pulse rounded-full bg-(--surface-highest)" aria-hidden="true"></span>{/if}
											</span>
											<Lock class="size-3.5 shrink-0 text-(--accent)" aria-hidden="true" />
										</div>
									</td>
									<td class="px-4 py-3 font-mono text-sm text-(--text)">{file.hydrated && file.size !== undefined ? formatBytes(file.size) : 'Encrypted'}</td>
									<td class="px-4 py-3 text-sm text-(--muted)">{relativeTime(file.updatedAt)}</td>
								</tr>
							{/each}
						{/if}
					</tbody>
				</table>
			</div>
		</section>
	</div>
</div>
