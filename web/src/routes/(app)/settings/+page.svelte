<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { onMount, tick } from 'svelte';
	import { Copy, LockKeyhole, Printer } from '@lucide/svelte';
	import Button from '#lib/components/Button.svelte';
	import InputField from '#lib/components/InputField.svelte';
	import Modal from '#lib/components/Modal.svelte';
	import S3StorageFields from '#lib/components/S3StorageFields.svelte';
	import { api } from '#lib/api.ts';
	import { clearThumbnailCache } from '#lib/thumbnail-cache.ts';
	import { formatBytes } from '#lib/utils/format.ts';
	import { toast } from '#lib/toast.ts';
	import { lockVault, setKeepVaultUnlocked, shouldKeepVaultUnlocked } from '#lib/vault/session.ts';
	import { cancelAndClearTransfers, useTransferManager } from '#lib/transfers/manager.ts';
	import { callVaultWorker } from '#lib/vault/worker-client.ts';
	import type { PageProps } from './$types';

	type SettingsTab = 'profile' | 'security' | 'storage';
	type StorageUsage = {
		usedBytes: number;
		reservedBytes: number;
		currentFileBytes: number;
		versionHistoryBytes: number;
		trashBytes: number;
		maxStorageBytes: number;
	};
	type StorageSettings = {
		maxStorageBytes: number;
		s3AccessKeyID: string;
		s3SecretAccessKeyConfigured: boolean;
		s3SessionTokenConfigured: boolean;
		s3Bucket: string;
		s3Endpoint: string;
		s3Region: string;
		s3UsePathStyle: boolean;
	};
	type RecoveryKeyDraft = {
		recoveryKey: string;
		encryptedMasterKeyRecovery: string;
		recoveryProofPublicKey: string;
	};
	const tabs: { id: SettingsTab; label: string }[] = [
		{ id: 'profile', label: 'Profile' },
		{ id: 'security', label: 'Security' },
		{ id: 'storage', label: 'Storage' },
	];

	let { data }: PageProps = $props();
	const transfers = useTransferManager();
	let activeTab = $state<SettingsTab>('profile');
	let storageUsage = $state<StorageUsage | null>(null);
	let storageLoading = $state(true);
	let storageError = $state('');
	let storageSettings = $state<StorageSettings | null>(null);
	let storageSettingsLoading = $state(true);
	let storageSettingsError = $state('');
	let savingStorageSettings = $state(false);
	let clearingCache = $state(false);
	let keepVaultOnRefresh = $state(false);
	let recoveryConfirmOpen = $state(false);
	let recoveryGenerating = $state(false);
	let recoveryActivating = $state(false);
	let recoveryAcknowledged = $state(false);
	let recoveryKeyActive = $state(false);
	let recoveryActivationUncertain = $state(false);
	let recoveryKeyDraft = $state<RecoveryKeyDraft | null>(null);
	let recoveryError = $state('');
	const totalStorageBytes = $derived(
		storageUsage ? storageUsage.usedBytes + storageUsage.reservedBytes : 0,
	);
	const storagePercent = $derived(
		storageUsage && storageUsage.maxStorageBytes > 0
			? Math.min(100, Math.round((totalStorageBytes / storageUsage.maxStorageBytes) * 100))
			: 0,
	);
	const storageBreakdown = $derived.by(() => {
		if (!storageUsage) return [];
		const scale = storageUsage.maxStorageBytes || totalStorageBytes;
		const parts = [
			{
				label: 'Current files',
				bytes: storageUsage.currentFileBytes,
				color: 'bg-(--accent)',
				available: false,
			},
			{
				label: 'Version history',
				bytes: storageUsage.versionHistoryBytes,
				color: 'bg-violet-500',
				available: false,
			},
			{
				label: 'Trash',
				bytes: storageUsage.trashBytes,
				color: 'bg-(--muted-strong)',
				available: false,
			},
			{
				label: 'Active uploads',
				bytes: storageUsage.reservedBytes,
				color: 'bg-amber-400',
				available: false,
			},
		];

		if (storageUsage.maxStorageBytes > 0) {
			parts.push({
				label: 'Available',
				bytes: Math.max(0, storageUsage.maxStorageBytes - totalStorageBytes),
				color: 'bg-(--surface-raised)',
				available: true,
			});
		}

		return parts.map((part) => ({
			...part,
			width: scale > 0 ? Math.min(100, (part.bytes / scale) * 100) : 0,
		}));
	});

	onMount(() => {
		keepVaultOnRefresh = shouldKeepVaultUnlocked();
		void loadStorageUsage();
		void loadStorageSettings();
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

	async function loadStorageSettings(): Promise<void> {
		storageSettingsLoading = true;
		storageSettingsError = '';
		try {
			const { data: settings } = await api.get<StorageSettings>('/api/settings/storage');
			storageSettings = settings;
		} catch (error) {
			storageSettingsError =
				error instanceof Error ? error.message : 'Could not load storage settings.';
		} finally {
			storageSettingsLoading = false;
		}
	}

	async function saveStorageSettings(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (savingStorageSettings || !(event.currentTarget instanceof HTMLFormElement)) return;
		const form = event.currentTarget;
		const values = new FormData(form);
		const value = (name: string) => String(values.get(name) ?? '');
		savingStorageSettings = true;
		storageSettingsError = '';
		try {
			const { data: settings } = await api.put<StorageSettings>('/api/settings/storage', {
				storageGB: value('storage_gb'),
				s3AccessKeyID: value('s3_access_key_id'),
				s3SecretAccessKey: value('s3_secret_access_key'),
				s3SessionToken: value('s3_session_token'),
				clearSessionToken: values.get('clear_session_token') === 'on',
				s3Bucket: value('s3_bucket'),
				s3Endpoint: value('s3_endpoint'),
				s3Region: value('s3_region'),
				s3UsePathStyle: values.get('s3_use_path_style') === 'on',
			});
			storageSettings = settings;
			if (storageUsage)
				storageUsage = { ...storageUsage, maxStorageBytes: settings.maxStorageBytes };
			await tick();
			const secretAccessKey = form.elements.namedItem('s3_secret_access_key');

			if (secretAccessKey instanceof HTMLInputElement) secretAccessKey.value = '';
			const sessionToken = form.elements.namedItem('s3_session_token');

			if (sessionToken instanceof HTMLInputElement) sessionToken.value = '';
			const clearSessionToken = form.elements.namedItem('clear_session_token');

			if (clearSessionToken instanceof HTMLInputElement) clearSessionToken.checked = false;
			toast.success('Storage settings saved.');
		} catch (error) {
			storageSettingsError =
				error instanceof Error ? error.message : 'Could not save storage settings.';
		} finally {
			savingStorageSettings = false;
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
		try {
			await cancelAndClearTransfers(transfers);
		} catch {
			toast.error('Could not cancel active transfers.');
		}
		await lockVault();
		await goto(resolve('/(app)/lock'));
	}

	async function generateRecoveryKey(): Promise<void> {
		if (recoveryGenerating) return;
		recoveryConfirmOpen = false;
		recoveryError = '';
		recoveryAcknowledged = false;
		recoveryKeyActive = false;
		recoveryActivationUncertain = false;
		recoveryGenerating = true;
		try {
			recoveryKeyDraft = await callVaultWorker('prepareVaultRecovery', { userID: data.user.id });
		} catch (error) {
			recoveryError = error instanceof Error ? error.message : 'Could not generate a recovery key.';
		} finally {
			recoveryGenerating = false;
		}
	}

	async function activateRecoveryKey(): Promise<void> {
		if (!recoveryKeyDraft || !recoveryAcknowledged || recoveryActivating) return;
		recoveryActivating = true;
		recoveryError = '';
		try {
			await api.put('/api/settings/recovery-key', {
				encryptedMasterKeyRecovery: recoveryKeyDraft.encryptedMasterKeyRecovery,
				recoveryProofPublicKey: recoveryKeyDraft.recoveryProofPublicKey,
			});
			recoveryKeyActive = true;
			recoveryActivationUncertain = false;
			toast.success('New vault recovery key activated.');
		} catch {
			recoveryActivationUncertain = true;
			recoveryError =
				'Activation could not be confirmed. Keep this key and retry; Arkive may already have activated it.';
		} finally {
			recoveryActivating = false;
		}
	}

	async function copyRecoveryKey(): Promise<void> {
		if (!recoveryKeyDraft) return;
		try {
			await navigator.clipboard.writeText(recoveryKeyDraft.recoveryKey);
			toast.success('Recovery key copied.');
		} catch {
			toast.error('Could not copy the recovery key.');
		}
	}

	function discardRecoveryKey(): void {
		if (recoveryActivating || recoveryActivationUncertain) return;
		recoveryKeyDraft = null;
		recoveryAcknowledged = false;
		recoveryKeyActive = false;
		recoveryActivationUncertain = false;
		recoveryError = '';
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

<section class="mx-auto w-full max-w-5xl px-5 py-7 min-[640px]:px-8 print:hidden">
	<header class="border-b border-(--border-soft) pb-5">
		<h1 class="m-0 text-2xl leading-8 font-semibold text-(--text)">Settings</h1>
		<p class="mt-1 mb-0 text-sm leading-5 text-(--muted)">
			Manage your account, security, and storage.
		</p>
	</header>

	<div
		class="flex gap-4 border-b border-(--border-soft) min-[640px]:gap-6"
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
				<div class="border-b border-(--border-soft) py-4">
					<div
						class="flex flex-col gap-4 min-[640px]:flex-row min-[640px]:items-start min-[640px]:justify-between"
					>
						<div>
							<h3 class="m-0 text-sm leading-5 font-medium text-(--text)">Vault recovery key</h3>
							<p class="mt-1 mb-0 text-sm leading-5 text-(--muted)">
								Forgot your password? Use this key with the reset link to set a new one and get back
								into your vault. Keep it somewhere safe and never share it. Activating a replacement
								makes this key stop working; changing your password does not.
							</p>
						</div>
						<Button
							text={recoveryGenerating ? 'Generating…' : 'Generate replacement key'}
							variant="secondary"
							disabled={recoveryGenerating || recoveryKeyDraft !== null}
							class="shrink-0"
							onclick={() => (recoveryConfirmOpen = true)}
						/>
					</div>
					{#if recoveryError && !recoveryKeyDraft}
						<p class="mt-3 mb-0 text-sm text-(--danger)" role="alert">{recoveryError}</p>
					{/if}
					{#if recoveryKeyDraft}
						<div class="mt-4 border-t border-(--border-soft) pt-4">
							<p class="m-0 text-sm leading-5 text-(--muted)" role="status">
								{recoveryKeyActive
									? 'Replacement key activated. The previous recovery key no longer works.'
									: 'Save this key before activating it. The previous key remains valid until activation.'}
							</p>
							<code
								class="mt-3 block rounded-md border border-(--border) bg-(--surface-raised) p-3 font-mono text-sm leading-6 break-all text-(--text)"
							>
								{recoveryKeyDraft.recoveryKey}
							</code>
							<div class="mt-3 flex flex-wrap gap-2">
								{#snippet copyRecoveryIcon()}<Copy class="size-4" aria-hidden="true" />{/snippet}
								<Button
									text="Copy key"
									variant="secondary"
									icon={copyRecoveryIcon}
									onclick={copyRecoveryKey}
								/>
								{#snippet printRecoveryIcon()}<Printer
										class="size-4"
										aria-hidden="true"
									/>{/snippet}
								<Button
									text="Print / Save as PDF"
									variant="secondary"
									icon={printRecoveryIcon}
									onclick={() => window.print()}
								/>
								{#if recoveryKeyActive}
									<Button text="Done" variant="secondary" onclick={discardRecoveryKey} />
								{:else}
									<Button
										text="Discard"
										variant="secondary"
										disabled={recoveryActivating || recoveryActivationUncertain}
										onclick={discardRecoveryKey}
									/>
								{/if}
							</div>
							{#if !recoveryKeyActive}
								<label class="mt-4 flex cursor-pointer items-start gap-3">
									<input
										class="mt-0.5 size-4 shrink-0 accent-(--accent)"
										type="checkbox"
										bind:checked={recoveryAcknowledged}
									/>
									<span class="text-sm leading-5 text-(--muted)">
										I have securely saved this new key. I understand activating it will permanently
										invalidate the previous key.
									</span>
								</label>
								{#if recoveryError}
									<p class="mt-3 mb-0 text-sm text-(--danger)" role="alert">{recoveryError}</p>
								{/if}
								<Button
									text="Activate replacement key"
									busyText="Activating…"
									busy={recoveryActivating}
									variant="primary"
									class="mt-4"
									disabled={!recoveryAcknowledged}
									onclick={activateRecoveryKey}
								/>
							{/if}
						</div>
					{/if}
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
							<h3 class="m-0 text-sm font-medium text-(--text)">Storage allocation</h3>
							<p class="m-0 text-sm text-(--muted)">
								{formatBytes(totalStorageBytes)}
								{storageUsage.maxStorageBytes > 0
									? `of ${formatBytes(storageUsage.maxStorageBytes)}`
									: '· no configured limit'}
							</p>
						</div>
						<div
							class="mt-3 flex h-2 overflow-hidden rounded-full bg-(--surface-raised)"
							role={storageUsage.maxStorageBytes > 0 ? 'progressbar' : 'img'}
							aria-label={storageUsage.maxStorageBytes > 0
								? 'Storage capacity used'
								: 'Storage breakdown'}
							aria-valuemin={storageUsage.maxStorageBytes > 0 ? 0 : undefined}
							aria-valuemax={storageUsage.maxStorageBytes > 0 ? 100 : undefined}
							aria-valuenow={storageUsage.maxStorageBytes > 0 ? storagePercent : undefined}
						>
							{#each storageBreakdown as part (part.label)}
								{#if !part.available && part.bytes > 0}
									<div class={`h-full shrink-0 ${part.color}`} style={`width:${part.width}%`}></div>
								{/if}
							{/each}
						</div>
						<div class="mt-3 grid grid-cols-1 gap-x-5 gap-y-2 text-xs sm:grid-cols-2">
							{#each storageBreakdown as part (part.label)}
								<div class="flex min-w-0 items-center justify-between gap-3">
									<span class="flex min-w-0 items-center gap-2 text-(--muted)">
										<span class={`size-2 shrink-0 rounded-full ${part.color}`} aria-hidden="true"
										></span>
										<span class="truncate">{part.label}</span>
									</span>
									<span class="shrink-0 text-(--text)">{formatBytes(part.bytes)}</span>
								</div>
							{/each}
						</div>
						<p class="mt-3 mb-0 text-xs text-(--muted)">
							Uploads reserve space while in progress. Trash remains counted until permanently
							deleted.
						</p>
					</div>
				{/if}
				<div class="border-b border-(--border-soft) py-4">
					<h3 class="m-0 text-sm leading-5 font-medium text-(--text)">S3-compatible storage</h3>
					<p class="mt-1 mb-4 text-sm leading-5 text-(--muted)">
						Edit the connection details saved during setup. Secret values stay on the server; leave
						their fields blank to keep them.
					</p>
					{#if storageSettingsLoading}
						<p class="m-0 text-sm text-(--muted)" role="status">Loading storage settings…</p>
					{:else if storageSettingsError && !storageSettings}
						<p class="m-0 text-sm text-(--danger)" role="alert">{storageSettingsError}</p>
						<Button text="Retry" variant="secondary" class="mt-3" onclick={loadStorageSettings} />
					{:else if storageSettings}
						<form class="grid gap-1" onsubmit={saveStorageSettings}>
							<InputField
								label="Storage limit in GB"
								name="storage_gb"
								type="number"
								min="0"
								step="1"
								required
								value={String(Math.floor(storageSettings.maxStorageBytes / 1073741824))}
								description="Use 0 for unlimited."
							/>
							<S3StorageFields
								required
								showCredentialStatus
								secretConfigured={storageSettings.s3SecretAccessKeyConfigured}
								sessionTokenConfigured={storageSettings.s3SessionTokenConfigured}
								allowClearSessionToken
								values={{
									endpoint: storageSettings.s3Endpoint,
									bucket: storageSettings.s3Bucket,
									region: storageSettings.s3Region,
									accessKeyID: storageSettings.s3AccessKeyID,
									usePathStyle: storageSettings.s3UsePathStyle,
								}}
							/>
							{#if storageSettingsError}
								<p class="m-0 text-sm text-(--danger)" role="alert">{storageSettingsError}</p>
							{/if}
							<Button
								text="Save storage settings"
								busyText="Saving…"
								busy={savingStorageSettings}
								variant="primary"
								type="submit"
								class="justify-self-start"
							/>
						</form>
					{/if}
				</div>
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

{#if recoveryKeyDraft}
	<section class="hidden min-h-screen bg-white p-8 text-black print:block">
		<h1 class="m-0 text-2xl font-semibold">Arkive Core vault recovery key</h1>
		<p class="mt-4 text-sm">
			Keep this key somewhere safe. It is the only way to recover your vault.
		</p>
		<p class="mt-8 break-all font-mono text-lg">{recoveryKeyDraft.recoveryKey}</p>
		<p class="mt-8 text-sm">Generated by Arkive Core.</p>
	</section>
{/if}

<Modal
	open={recoveryConfirmOpen}
	label="Generate replacement recovery key"
	title="Generate a replacement key?"
	onClose={() => (recoveryConfirmOpen = false)}
>
	<p class="m-0 text-sm leading-5 text-(--muted)">
		Arkive cannot show your current key. A replacement will be generated in this browser. Save it
		securely and never share it. Your current key remains valid until you activate the replacement.
	</p>
	{#snippet actions()}
		<Button text="Cancel" variant="secondary" onclick={() => (recoveryConfirmOpen = false)} />
		<Button text="Generate key" variant="primary" onclick={generateRecoveryKey} />
	{/snippet}
</Modal>
