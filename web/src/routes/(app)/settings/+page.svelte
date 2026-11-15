<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { LockKeyhole } from '@lucide/svelte';
	import Button from '#lib/components/Button.svelte';
	import { api } from '#lib/api.ts';
	import { clearThumbnailCache } from '#lib/thumbnail-cache.ts';
	import { formatBytes } from '#lib/utils/format.ts';
	import { toast } from '#lib/toast.ts';
	import { lockVault, setKeepVaultUnlocked, shouldKeepVaultUnlocked } from '#lib/vault/session.ts';
	import type { PageProps } from './$types';

	type SettingsTab = 'profile' | 'security' | 'storage';
	type StorageUsage = {
		usedBytes: number;
		reservedBytes: number;
		maxStorageBytes: number;
	};
	const tabs: { id: SettingsTab; label: string }[] = [
		{ id: 'profile', label: 'Profile' },
		{ id: 'security', label: 'Security' },
		{ id: 'storage', label: 'Storage' },
	];

	let { data }: PageProps = $props();
	let activeTab = $state<SettingsTab>('profile');
	let storageUsage = $state<StorageUsage | null>(null);
	let storageLoading = $state(true);
	let storageError = $state('');
	let clearingCache = $state(false);
	let keepVaultOnRefresh = $state(false);
	const totalStorageBytes = $derived(
		storageUsage ? storageUsage.usedBytes + storageUsage.reservedBytes : 0,
	);
	const storagePercent = $derived(
		storageUsage && storageUsage.maxStorageBytes > 0
			? Math.min(100, Math.round((totalStorageBytes / storageUsage.maxStorageBytes) * 100))
			: 0,
	);

	onMount(() => {
		keepVaultOnRefresh = shouldKeepVaultUnlocked();
		void loadStorageUsage();
	});

	async function loadStorageUsage(): Promise<void> {
		storageLoading = true;
		storageError = '';
		try {
			const { data: usage } = await api.get<StorageUsage>('/api/settings/storage-usage');
			storageUsage = usage;
		} catch (error) {
			storageError = error instanceof Error ? error.message : 'Could not load storage usage.';
		} finally {
			storageLoading = false;
		}
	}

	async function clearCache(): Promise<void> {
		if (clearingCache) return;
		clearingCache = true;
		try {
			await clearThumbnailCache();
			toast.success('Thumbnail cache cleared.');
		} catch {
			toast.error('Could not clear the thumbnail cache.');
		} finally {
			clearingCache = false;
		}
	}

	function handleTabKeydown(event: KeyboardEvent): void {
		const currentIndex = tabs.findIndex((tab) => tab.id === activeTab);
		let nextIndex = currentIndex;

		if (event.key === 'ArrowRight') nextIndex = (currentIndex + 1) % tabs.length;
		else if (event.key === 'ArrowLeft') nextIndex = (currentIndex + tabs.length - 1) % tabs.length;
		else if (event.key === 'Home') nextIndex = 0;
		else if (event.key === 'End') nextIndex = tabs.length - 1;
		else return;

		event.preventDefault();
		activeTab = tabs[nextIndex].id;
		requestAnimationFrame(() => document.getElementById(`settings-tab-${activeTab}`)?.focus());
	}

	async function lockCurrentVault(): Promise<void> {
		await lockVault();
		await goto(resolve('/(app)/lock'));
	}

	async function updateKeepVaultOnRefresh(event: Event): Promise<void> {
		const enabled = (event.currentTarget as HTMLInputElement).checked;

		try {
			await setKeepVaultUnlocked(enabled, data.user.id);
			keepVaultOnRefresh = enabled;
		} catch {
			keepVaultOnRefresh = false;
			toast.error('Could not save this preference in browser storage.');
		}
	}
</script>

<svelte:head>
	<title>Settings | Arkive</title>
</svelte:head>

<section class="mx-auto w-full max-w-5xl px-5 py-7 min-[640px]:px-8">
	<header class="border-b border-(--border-soft) pb-5">
		<h1 class="m-0 text-2xl leading-8 font-semibold text-(--text)">Settings</h1>
		<p class="mt-1 mb-0 text-sm leading-5 text-(--muted)">
			Manage your account, security, and storage.
		</p>
	</header>

	<div
		class="flex gap-6 overflow-x-auto border-b border-(--border-soft)"
		role="tablist"
		aria-label="Settings"
		tabindex="-1"
		onkeydown={handleTabKeydown}
	>
		{#each tabs as tab (tab.id)}
			<button
				id={`settings-tab-${tab.id}`}
				class={`-mb-px min-h-12 shrink-0 cursor-pointer border-b-2 px-1 text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-(--accent) ${activeTab === tab.id ? 'border-(--accent) text-(--text)' : 'border-transparent text-(--muted) hover:text-(--text)'}`}
				type="button"
				role="tab"
				aria-selected={activeTab === tab.id}
				aria-controls="settings-panel"
				tabindex={activeTab === tab.id ? 0 : -1}
				onclick={() => (activeTab = tab.id)}
			>
				{tab.label}
			</button>
		{/each}
	</div>

	<div
		id="settings-panel"
		class="py-6"
		role="tabpanel"
		aria-labelledby={`settings-tab-${activeTab}`}
		tabindex="0"
	>
		{#if activeTab === 'profile'}
			<div class="max-w-2xl">
				<h2 class="m-0 text-lg leading-6 font-semibold text-(--text)">Profile</h2>
				<p class="mt-1 mb-5 text-sm leading-5 text-(--muted)">Your Arkive account details.</p>
				<dl class="m-0">
					{#each [{ label: 'Name', value: data.user.name }, { label: 'Username', value: data.user.username }, { label: 'Email', value: data.user.email }] as field (field.label)}
						<div
							class="grid gap-1 border-b border-(--border-soft) py-4 min-[640px]:grid-cols-[160px_1fr] min-[640px]:gap-6"
						>
							<dt class="text-sm text-(--muted)">{field.label}</dt>
							<dd class="m-0 min-w-0 break-words text-sm text-(--text)">{field.value || '—'}</dd>
						</div>
					{/each}
				</dl>
			</div>
		{:else if activeTab === 'security'}
			<div class="max-w-2xl">
				<h2 class="m-0 text-lg leading-6 font-semibold text-(--text)">Security</h2>
				<p class="mt-1 mb-5 text-sm leading-5 text-(--muted)">
					Your vault is encrypted and unlocked only in this browser.
				</p>
				<div
					class="flex flex-col gap-4 border-b border-(--border-soft) py-4 min-[640px]:flex-row min-[640px]:items-center min-[640px]:justify-between"
				>
					<div class="flex items-start gap-3">
						<LockKeyhole class="mt-0.5 size-5 shrink-0 text-(--muted)" aria-hidden="true" />
						<div>
							<h3 class="m-0 text-sm leading-5 font-medium text-(--text)">Lock this device</h3>
							<p class="mt-1 mb-0 text-sm leading-5 text-(--muted)">
								Lock your vault until you unlock it again with your password.
							</p>
						</div>
					</div>
					<Button
						text="Lock vault"
						variant="secondary"
						class="shrink-0"
						onclick={lockCurrentVault}
					/>
				</div>
				<div class="border-b border-(--border-soft) py-4">
					<label class="flex cursor-pointer items-start gap-3">
						<input
							class="mt-0.5 size-4 shrink-0 accent-(--accent)"
							type="checkbox"
							checked={keepVaultOnRefresh}
							onchange={updateKeepVaultOnRefresh}
						/>
						<span>
							<span class="block text-sm leading-5 font-medium text-(--text)"
								>Keep vault unlocked after refresh</span
							>
							<span class="mt-1 block text-sm leading-5 text-(--muted)">
								Save the unlocked vault key in this tab's session storage. It is cleared when you
								lock the vault or log out.
							</span>
						</span>
					</label>
					<p class="mt-3 mb-0 pl-7 text-xs leading-4.5 text-(--muted-strong)">
						Any script running on this site can access session storage. Leave this off on shared or
						untrusted devices.
					</p>
				</div>
			</div>
		{:else}
			<div class="max-w-2xl">
				<h2 class="m-0 text-lg leading-6 font-semibold text-(--text)">Storage</h2>
				<p class="mt-1 mb-5 text-sm leading-5 text-(--muted)">
					View your vault usage and manage local thumbnail data.
				</p>
				{#if storageLoading}
					<p class="m-0 border-b border-(--border-soft) py-4 text-sm text-(--muted)" role="status">
						Loading storage usage…
					</p>
				{:else if storageError}
					<div class="border-b border-(--border-soft) py-4">
						<p class="m-0 text-sm text-(--danger)" role="alert">{storageError}</p>
						<Button text="Retry" variant="secondary" class="mt-3" onclick={loadStorageUsage} />
					</div>
				{:else if storageUsage}
					<div class="border-b border-(--border-soft) py-4">
						<div class="flex flex-wrap items-baseline justify-between gap-2">
							<h3 class="m-0 text-sm font-medium text-(--text)">Storage used</h3>
							<p class="m-0 text-sm text-(--muted)">
								{formatBytes(totalStorageBytes)}
								{storageUsage.maxStorageBytes > 0
									? `of ${formatBytes(storageUsage.maxStorageBytes)}`
									: '· no configured limit'}
							</p>
						</div>
						{#if storageUsage.maxStorageBytes > 0}
							<div
								class="mt-3 h-2 overflow-hidden rounded-full bg-(--surface-raised)"
								role="progressbar"
								aria-label="Storage used"
								aria-valuemin="0"
								aria-valuemax="100"
								aria-valuenow={storagePercent}
							>
								<div
									class="h-full rounded-full bg-(--accent)"
									style={`width:${storagePercent}%`}
								></div>
							</div>
						{/if}
						{#if storageUsage.reservedBytes > 0}
							<p class="mt-2 mb-0 text-xs text-(--muted)">
								{formatBytes(storageUsage.reservedBytes)} reserved for active uploads.
							</p>
						{/if}
					</div>
				{/if}
				<div
					class="flex flex-col gap-4 border-b border-(--border-soft) py-4 min-[640px]:flex-row min-[640px]:items-center min-[640px]:justify-between"
				>
					<div>
						<h3 class="m-0 text-sm leading-5 font-medium text-(--text)">Thumbnail cache</h3>
						<p class="mt-1 mb-0 text-sm leading-5 text-(--muted)">
							Remove locally cached encrypted thumbnails. They will download again when needed.
						</p>
					</div>
					<Button
						text="Clear cache"
						variant="secondary"
						busyText="Clearing…"
						busy={clearingCache}
						class="shrink-0"
						onclick={clearCache}
					/>
				</div>
			</div>
		{/if}
	</div>
</section>
