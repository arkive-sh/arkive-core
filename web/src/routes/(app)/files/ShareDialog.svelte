<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { Copy, ExternalLink, Link2, ShieldOff } from '@lucide/svelte';
	import axios from 'axios';
	import Drawer from '#lib/components/Drawer.svelte';
	import FileTypeIcon from '#lib/components/FileTypeIcon.svelte';
	import { api } from '#lib/api.ts';
	import { buildShareURL, decryptShareURL } from '#lib/share-link.ts';
	import { toast } from '#lib/toast.ts';
	import { toBase64Url } from '#lib/vault/bytes.ts';
	import { prepareShare } from '#lib/vault/shares.ts';
	import type { FileManagerEntry } from './types';

	type ShareRecord = {
		id: string;
		fileId: string;
		token: string;
		encryptedShareKey: string;
		allowPreview: boolean;
		allowDownload: boolean;
		burnAfterRead: boolean;
		expiresAt: string | null;
		hasPassword: boolean;
		status: 'active' | 'revoked' | 'expired' | 'burned';
		isExpired: boolean;
	};

	let {
		entry,
		vaultId,
		onClose,
	}: {
		entry: FileManagerEntry;
		vaultId: string;
		onClose: () => void;
	} = $props();
	let share = $state<ShareRecord | null>(null);
	let link = $state('');
	let allowPreview = $state(true);
	let allowDownload = $state(true);
	let expiresAt = $state('');
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let confirmRevoke = $state(false);

	function toLocalDateTime(value: string | null): string {
		if (!value) return '';
		const date = new Date(value);

		if (Number.isNaN(date.getTime())) return '';

		return new Date(date.getTime() - date.getTimezoneOffset() * 60_000).toISOString().slice(0, 16);
	}

	async function restoreLink(record: ShareRecord): Promise<void> {
		link = await decryptShareURL(window.location.origin, record.token, record.encryptedShareKey);
	}

	function statusLabel(): string {
		if (!share) return '';
		if (share.isExpired || share.status === 'expired') return 'Expired';
		if (share.status === 'revoked') return 'Revoked';
		if (share.status === 'burned') return 'Used';

		return 'Active';
	}

	function expirySummary(): string {
		if (!share?.expiresAt) return 'No expiry';
		const date = new Date(share.expiresAt);

		return Number.isNaN(date.getTime())
			? 'Expiry date unavailable'
			: `Expires ${new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date)}`;
	}

	function openLink(): void {
		if (!link || !share || share.status !== 'active' || share.isExpired) return;
		window.open(link, '_blank', 'noopener,noreferrer');
	}

	async function loadShare(): Promise<void> {
		loading = true;
		error = '';
		try {
			const { data } = await api.get<ShareRecord>(
				`/api/files/${encodeURIComponent(entry.id)}/share`,
			);
			share = data;
			allowPreview = data.allowPreview;
			allowDownload = data.allowDownload;
			expiresAt = toLocalDateTime(data.expiresAt);
			await restoreLink(data);
		} catch (cause) {
			if (axios.isAxiosError(cause) && cause.response?.status === 404) {
				share = null;

				return;
			}
			error = cause instanceof Error ? cause.message : 'Could not load this share.';
		} finally {
			loading = false;
		}
	}

	function expiryValue(): string {
		if (!expiresAt) return '';
		const date = new Date(expiresAt);

		return Number.isNaN(date.getTime()) ? '' : date.toISOString();
	}

	async function saveShare(): Promise<void> {
		if (saving) return;
		if (!allowPreview && !allowDownload) {
			error = 'Enable preview or download to create a usable link.';

			return;
		}
		if (!share && !entry.encryptedFileKey) {
			error = 'The encrypted file key is not loaded yet. Close this dialog and try again.';

			return;
		}

		saving = true;
		error = '';
		try {
			if (share) {
				const { data } = await api.patch<ShareRecord>(
					`/api/shares/${encodeURIComponent(share.id)}`,
					{
						expiresAt: expiryValue(),
						allowPreview,
						allowDownload,
						requirePassword: share.hasPassword,
						burnAfterRead: share.burnAfterRead,
					},
				);
				share = data;
				expiresAt = toLocalDateTime(data.expiresAt);
				toast.success('Share settings updated.');
			} else {
				const random = crypto.getRandomValues(new Uint8Array(32));
				const token = toBase64Url(random);
				random.fill(0);
				const prepared = await prepareShare(
					{
						vaultId,
						fileId: entry.id,
						folderId: entry.parentFolderId ?? null,
						encryptedFileKey: entry.encryptedFileKey!,
					},
					token,
				);

				try {
					const { data } = await api.post<ShareRecord>(
						`/api/files/${encodeURIComponent(entry.id)}/share`,
						{
							token,
							expiresAt: expiryValue(),
							allowPreview,
							allowDownload,
							encryptedShareKey: prepared.encryptedShareKey,
							encryptedFileKeyForShare: prepared.encryptedFileKeyForShare,
						},
					);
					share = data;
					link = buildShareURL(window.location.origin, token, prepared.shareSecret);
					toast.success('Encrypted share link created.');
				} finally {
					prepared.shareSecret.fill(0);
				}
			}
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not save share settings.';
		} finally {
			saving = false;
		}
	}

	async function changeShareStatus(action: 'revoke' | 'activate'): Promise<void> {
		if (!share || saving) return;
		saving = true;
		error = '';
		try {
			const { data } = await api.post<ShareRecord>(
				`/api/shares/${encodeURIComponent(share.id)}/${action}`,
			);
			share = data;
			confirmRevoke = false;
			toast.success(action === 'revoke' ? 'Share link revoked.' : 'Share link reactivated.');
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not change this share link.';
		} finally {
			saving = false;
		}
	}

	async function copyLink(): Promise<void> {
		if (!link) return;
		try {
			await navigator.clipboard.writeText(link);
			toast.success('Share link copied.');
		} catch {
			toast.error('Could not copy the link.');
		}
	}

	function setPermission(
		permission: 'preview' | 'download',
		enabled: boolean,
		input: HTMLInputElement,
	): void {
		const nextPreview = permission === 'preview' ? enabled : allowPreview;
		const nextDownload = permission === 'download' ? enabled : allowDownload;

		if (!nextPreview && !nextDownload) {
			input.checked = true;
			error = 'At least one of preview or download must stay enabled.';

			return;
		}
		allowPreview = nextPreview;
		allowDownload = nextDownload;
		error = '';
	}

	onMount(() => void loadShare());
	onDestroy(() => {
		link = '';
	});
</script>

<Drawer open={true} label={`Share ${entry.name}`} title="Share" {onClose}>
	<div class="grid gap-6">
		<div class="flex min-w-0 items-center gap-3 border-b border-(--border) pb-4">
			<FileTypeIcon name={entry.name} mime={entry.type} className="size-5 shrink-0" />
			<span class="min-w-0 truncate text-sm font-medium" title={entry.name}>{entry.name}</span>
		</div>

		{#if loading}
			<p class="m-0 text-sm text-(--muted)" role="status">Loading share settings…</p>
		{:else}
			{#if share}
				<section class="grid gap-3 border-b border-(--border) pb-5">
					<div class="flex items-start justify-between gap-3">
						<div>
							<h3 class="m-0 text-sm font-semibold">Share link</h3>
							<p class="m-0 mt-1 text-xs text-(--muted)">{expirySummary()}</p>
						</div>
						<span
							class={`inline-flex items-center gap-1.5 text-xs font-medium ${share.isExpired || share.status === 'expired' ? 'text-amber-300' : share.status === 'active' ? 'text-emerald-300' : 'text-(--muted)'}`}
						>
							<span
								class={`size-2 rounded-full ${share.isExpired || share.status === 'expired' ? 'bg-amber-300' : share.status === 'active' ? 'bg-emerald-300' : 'bg-(--muted)'}`}
							></span>
							{statusLabel()}
						</span>
					</div>
					{#if link && share.status !== 'burned'}
						<div class="flex flex-wrap gap-2">
							<button
								class="inline-flex min-h-10 cursor-pointer items-center gap-2 rounded-md bg-(--accent) px-3.5 text-sm font-semibold text-(--accent-ink) hover:bg-(--accent-strong)"
								type="button"
								onclick={() => void copyLink()}
							>
								<Copy class="size-4" aria-hidden="true" />Copy link
							</button>
							<button
								class="inline-flex min-h-10 cursor-pointer items-center gap-2 rounded-md border border-(--border) px-3.5 text-sm hover:bg-(--surface-raised) disabled:cursor-not-allowed disabled:opacity-50"
								type="button"
								disabled={share.status !== 'active' || share.isExpired}
								onclick={openLink}
							>
								<ExternalLink class="size-4" aria-hidden="true" />Open
							</button>
						</div>
					{:else if share.status === 'burned'}
						<p class="m-0 text-sm text-(--muted)">This one-time link has already been used.</p>
					{/if}
					{#if share.status === 'revoked'}
						<button
							class="inline-flex min-h-10 cursor-pointer items-center justify-center gap-2 rounded-md border border-(--border) px-3.5 text-sm font-medium hover:bg-(--surface-raised) disabled:cursor-wait disabled:opacity-50"
							type="button"
							disabled={saving}
							onclick={() => void changeShareStatus('activate')}
						>
							<Link2 class="size-4" aria-hidden="true" />Reactivate link
						</button>
					{/if}
				</section>
			{/if}

			<fieldset
				class="grid gap-1 border-b border-(--border) p-0 pb-5"
				disabled={saving || share?.status === 'revoked' || share?.status === 'burned'}
			>
				<legend class="mb-1 text-sm font-semibold">Permissions</legend>
				<label class="flex cursor-pointer items-start gap-3 py-2">
					<input
						class="mt-1 size-4 accent-(--accent)"
						type="checkbox"
						checked={allowPreview}
						onchange={(event) =>
							setPermission('preview', event.currentTarget.checked, event.currentTarget)}
					/>
					<span class="grid gap-0.5">
						<span class="text-sm font-medium">Preview</span>
						<span class="text-xs leading-5 text-(--muted)">Open supported files in Arkive.</span>
					</span>
				</label>
				<label class="flex cursor-pointer items-start gap-3 py-2">
					<input
						class="mt-1 size-4 accent-(--accent)"
						type="checkbox"
						checked={allowDownload}
						onchange={(event) =>
							setPermission('download', event.currentTarget.checked, event.currentTarget)}
					/>
					<span class="grid gap-0.5">
						<span class="text-sm font-medium">Download</span>
						<span class="text-xs leading-5 text-(--muted)">Save a decrypted copy.</span>
					</span>
				</label>
				<p class="m-0 text-xs text-(--muted)">At least one permission must stay enabled.</p>
			</fieldset>

			<label
				class="grid gap-2 border-b border-(--border) pb-5 text-sm font-medium"
				for="share-expiry"
			>
				<span>Expires</span>
				<span class="text-xs font-normal text-(--muted)">Leave empty for no expiry.</span>
				<input
					id="share-expiry"
					class="rounded-md border border-(--border) bg-(--surface-raised) px-3 py-2.5 text-sm"
					type="datetime-local"
					bind:value={expiresAt}
					disabled={saving || share?.status === 'revoked' || share?.status === 'burned'}
				/>
			</label>

			{#if share && share.status !== 'revoked' && share.status !== 'burned'}
				<div>
					{#if confirmRevoke}
						<div class="grid gap-3">
							<p class="m-0 text-sm text-(--muted)">
								Anyone with this link will lose access immediately.
							</p>
							<div class="flex gap-2">
								<button
									class="cursor-pointer rounded-md border border-(--border) px-3 py-2 text-sm"
									type="button"
									disabled={saving}
									onclick={() => (confirmRevoke = false)}>Cancel</button
								>
								<button
									class="inline-flex cursor-pointer items-center gap-2 rounded-md border border-red-500/50 px-3 py-2 text-sm text-red-400 disabled:cursor-wait disabled:opacity-50"
									type="button"
									disabled={saving}
									onclick={() => void changeShareStatus('revoke')}
								>
									<ShieldOff class="size-4" aria-hidden="true" />Confirm revoke
								</button>
							</div>
						</div>
					{:else}
						<button
							class="inline-flex cursor-pointer items-center gap-2 text-sm text-(--muted) hover:text-red-400"
							type="button"
							onclick={() => (confirmRevoke = true)}
						>
							<ShieldOff class="size-4" aria-hidden="true" />Revoke link
						</button>
					{/if}
				</div>
			{/if}
		{/if}

		{#if error}<p class="m-0 text-sm text-red-400" role="alert">{error}</p>{/if}
	</div>
	{#snippet actions()}
		{#if !loading && share?.status !== 'revoked' && share?.status !== 'burned'}
			<button
				class="min-h-10 cursor-pointer rounded-md bg-(--accent) px-4 text-sm font-semibold text-(--accent-ink) hover:bg-(--accent-strong) disabled:cursor-not-allowed disabled:opacity-50"
				type="button"
				disabled={saving || (!allowPreview && !allowDownload)}
				onclick={() => void saveShare()}
				>{saving ? 'Saving…' : share ? 'Save changes' : 'Create link'}</button
			>
		{/if}
	{/snippet}
</Drawer>
