<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { Copy, Link2, ShieldOff } from '@lucide/svelte';
	import axios from 'axios';
	import Modal from '#lib/components/Modal.svelte';
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

<Modal open={true} label={`Share ${entry.name}`} title="Share file" {onClose}>
	<div class="grid gap-4">
		<p class="m-0 truncate font-medium" title={entry.name}>{entry.name}</p>
		{#if loading}
			<p class="m-0 text-(--muted)">Loading share settings…</p>
		{:else}
			<fieldset class="grid gap-3 border-0 p-0">
				<legend class="mb-2 text-sm font-semibold">Link access</legend>
				<label class="flex items-start gap-2.5">
					<input
						class="mt-1 accent-(--accent)"
						type="checkbox"
						checked={allowPreview}
						onchange={(event) =>
							setPermission('preview', event.currentTarget.checked, event.currentTarget)}
					/>
					<span
						><span class="block">Preview</span><span class="text-xs text-(--muted)"
							>Open supported files in Arkive.</span
						></span
					>
				</label>
				<label class="flex items-start gap-2.5">
					<input
						class="mt-1 accent-(--accent)"
						type="checkbox"
						checked={allowDownload}
						onchange={(event) =>
							setPermission('download', event.currentTarget.checked, event.currentTarget)}
					/>
					<span
						><span class="block">Download</span><span class="text-xs text-(--muted)"
							>Save a decrypted copy.</span
						></span
					>
				</label>
			</fieldset>

			<label class="grid gap-1.5 text-sm font-medium" for="share-expiry">
				Link expiry <span class="text-xs font-normal text-(--muted)"
					>Leave empty for no expiry.</span
				>
				<input
					id="share-expiry"
					class="rounded border border-(--border) bg-(--surface-raised) px-3 py-2"
					type="datetime-local"
					bind:value={expiresAt}
				/>
			</label>

			{#if share}
				<p class="m-0 text-xs text-(--muted)">
					Status: {share.isExpired ? 'Expired' : share.status}
				</p>
				{#if link}
					<div class="flex min-w-0 gap-2">
						<input
							class="min-w-0 flex-1 rounded border border-(--border) bg-(--surface-raised) px-3 py-2 text-xs"
							readonly
							value={link}
							aria-label="Share link"
						/>
						<button
							class="inline-flex cursor-pointer items-center gap-2 rounded border border-(--border) px-3 py-2"
							type="button"
							onclick={() => void copyLink()}
							aria-label="Copy share link"><Copy class="size-4" aria-hidden="true" /></button
						>
					</div>
				{/if}
				{#if share.status === 'revoked'}
					<button
						class="inline-flex cursor-pointer items-center gap-2 justify-self-start rounded border border-(--border) px-3 py-2 text-sm"
						type="button"
						disabled={saving}
						onclick={() => void changeShareStatus('activate')}
						><Link2 class="size-4" aria-hidden="true" />Reactivate link</button
					>
				{:else if share.status !== 'burned' && confirmRevoke}
					<p class="m-0 text-sm">Anyone with this link will lose access. Revoke it?</p>
					<button
						class="inline-flex cursor-pointer items-center gap-2 justify-self-start rounded border border-red-500 px-3 py-2 text-sm text-red-400"
						type="button"
						disabled={saving}
						onclick={() => void changeShareStatus('revoke')}
						><ShieldOff class="size-4" aria-hidden="true" />Confirm revoke</button
					>
				{:else if share.status !== 'burned'}
					<button
						class="inline-flex cursor-pointer items-center gap-2 justify-self-start rounded border border-(--border) px-3 py-2 text-sm"
						type="button"
						onclick={() => (confirmRevoke = true)}
						><ShieldOff class="size-4" aria-hidden="true" />Revoke link</button
					>
				{/if}
			{/if}
		{/if}
		{#if error}<p class="m-0 text-sm text-red-400" role="alert">{error}</p>{/if}
	</div>
	{#snippet actions()}
		<button
			class="cursor-pointer rounded border border-(--border) px-4 py-2 text-sm"
			type="button"
			onclick={onClose}>Close</button
		>
		{#if !loading && share?.status !== 'revoked' && share?.status !== 'burned'}
			<button
				class="cursor-pointer rounded bg-(--accent) px-4 py-2 text-sm font-semibold text-white disabled:cursor-not-allowed disabled:opacity-50"
				type="button"
				disabled={saving || (!allowPreview && !allowDownload)}
				onclick={() => void saveShare()}
				>{saving ? 'Saving…' : share ? 'Save settings' : 'Create link'}</button
			>
		{/if}
	{/snippet}
</Modal>
