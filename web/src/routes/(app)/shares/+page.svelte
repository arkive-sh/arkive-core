<script lang="ts">
	import { page } from '$app/state';
	import { resolve } from '$app/paths';
	import { tick } from 'svelte';
	import { Copy, Download, ExternalLink, Eye, EyeOff, Settings, X } from '@lucide/svelte';
	import axios from 'axios';
	import { api } from '#lib/api.ts';
	import FileTypeIcon from '#lib/components/FileTypeIcon.svelte';
	import Tooltip from '#lib/components/Tooltip.svelte';
	import { decryptShareURL } from '#lib/share-link.ts';
	import { toast } from '#lib/toast.ts';
	import { decryptSearchFile } from '#lib/vault/search.ts';
	import { vaultUnlocked } from '#lib/vault/state.ts';
	import type { EncryptedSearchFile } from '#lib/vault/types.ts';
	import ShareDialog from '../files/ShareDialog.svelte';
	import type { FileManagerEntry } from '../files/types';

	type ApiShare = {
		id: string;
		fileId: string;
		vaultId: string;
		parentFolderId: string | null;
		encryptedMetadata: string;
		encryptedFileKey: string;
		allowPreview: boolean;
		allowDownload: boolean;
		burnAfterRead: boolean;
		expiresAt: string | null;
		status: 'active' | 'revoked' | 'expired' | 'burned';
		isExpired: boolean;
		createdAt: string;
		updatedAt: string;
		isTrashed: boolean;
	};
	type ShareCryptoRecord = { token: string; encryptedShareKey: string };
	type ListedShare = { record: ApiShare; entry: FileManagerEntry };
	type ShareAction = { id: string; kind: 'copy' | 'open' };

	let shares = $state<ListedShare[]>([]);
	let selectedShare = $state<ListedShare | null>(null);
	let actionInProgress = $state<ShareAction | null>(null);
	let loading = $state(true);
	let error = $state('');
	let requestID = 0;
	const focusFileId = $derived(page.url.searchParams.get('focus_file'));
	const focusedShareId = $derived(
		shares.find((share) => share.entry.id === focusFileId && canOpen(share.record))?.record.id ??
			shares.find((share) => share.entry.id === focusFileId)?.record.id ??
			null,
	);

	async function loadShares(): Promise<void> {
		const request = ++requestID;
		loading = true;
		error = '';

		try {
			const { data } = await api.get<{ shares: ApiShare[] }>('/api/shares');
			const decrypted = await Promise.all(
				data.shares.map(async (record) => {
					const file: EncryptedSearchFile = {
						id: record.fileId,
						folderId: record.parentFolderId,
						vaultId: record.vaultId,
						encryptedMetadata: record.encryptedMetadata,
						encryptedFileKey: record.encryptedFileKey,
						score: 0,
						url: '',
					};
					let name = `File ${record.fileId.slice(0, 8)}`;
					let type = 'application/octet-stream';

					try {
						const { metadata } = await decryptSearchFile(file);

						if (typeof metadata.name === 'string') name = metadata.name;
						if (typeof metadata.mime === 'string') type = metadata.mime;
					} catch {
						// Keep the link manageable if its encrypted file metadata cannot be opened.
					}

					return {
						record,
						entry: {
							id: record.fileId,
							kind: 'file',
							name,
							type,
							size: null,
							updatedAt: record.updatedAt,
							parentFolderId: record.parentFolderId,
							encryptedFileKey: record.encryptedFileKey,
						},
					} satisfies ListedShare;
				}),
			);

			if (request === requestID) shares = decrypted;
		} catch (cause) {
			if (request !== requestID) return;
			shares = [];
			error = axios.isAxiosError(cause)
				? cause.message
				: cause instanceof Error
					? cause.message
					: 'Could not load shared links.';
		} finally {
			if (request === requestID) loading = false;
		}
	}

	function statusLabel(share: ApiShare): string {
		if (share.isTrashed) return 'In Trash';
		if (share.isExpired || share.status === 'expired') return 'Expired';
		if (share.status === 'burned') return 'Used';
		if (share.status === 'revoked') return 'Revoked';

		return 'Active';
	}

	function statusClass(share: ApiShare): string {
		if (share.isTrashed) return 'border-(--border) bg-(--surface-raised) text-(--muted)';
		if (share.isExpired || share.status === 'expired')
			return 'border-amber-500/40 bg-amber-500/10 text-amber-300';
		if (share.status === 'burned') return 'border-(--border) bg-(--surface-raised) text-(--muted)';
		if (share.status === 'revoked') return 'border-(--border) bg-(--surface-raised) text-(--muted)';

		return 'border-emerald-500/40 bg-emerald-500/10 text-emerald-300';
	}

	function expiryLabel(value: string | null): string {
		if (!value) return 'Never';
		const date = new Date(value);

		return Number.isNaN(date.getTime())
			? 'Date unavailable'
			: new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(
					date,
				);
	}

	function canOpen(share: ApiShare): boolean {
		return !share.isTrashed && !share.isExpired && share.status === 'active';
	}

	function fileLocation(share: ListedShare): string {
		const route = resolve(share.record.isTrashed ? '/(app)/trash' : '/(app)');
		const folder = share.record.parentFolderId
			? `folder_id=${encodeURIComponent(share.record.parentFolderId)}&`
			: '';

		return `${route}?${folder}focus_file=${encodeURIComponent(share.entry.id)}`;
	}

	async function resolveShareURL(shareID: string): Promise<string> {
		const { data } = await api.get<ShareCryptoRecord>(
			`/api/shares/${encodeURIComponent(shareID)}/crypto-record`,
		);

		return decryptShareURL(window.location.origin, data.token, data.encryptedShareKey);
	}

	async function copyLink(share: ListedShare): Promise<void> {
		if (share.record.isTrashed || actionInProgress) return;
		actionInProgress = { id: share.record.id, kind: 'copy' };

		try {
			await navigator.clipboard.writeText(await resolveShareURL(share.record.id));
			toast.success('Share link copied.');
		} catch {
			toast.error('Could not copy the share link.');
		} finally {
			actionInProgress = null;
		}
	}

	function openLink(share: ListedShare): void {
		if (!canOpen(share.record) || actionInProgress) return;

		const popup = window.open('about:blank', '_blank');

		if (!popup) {
			toast.error('Allow pop-ups to open this share link.');

			return;
		}
		popup.opener = null;
		actionInProgress = { id: share.record.id, kind: 'open' };

		void (async () => {
			try {
				popup.location.replace(await resolveShareURL(share.record.id));
			} catch {
				popup.close();
				toast.error('Could not open the share link.');
			} finally {
				actionInProgress = null;
			}
		})();
	}

	function closeShare(): void {
		selectedShare = null;
		void loadShares();
	}

	$effect(() => {
		if ($vaultUnlocked) void loadShares();
	});

	$effect(() => {
		const id = focusedShareId;
		if (!id || loading) return;

		let canceled = false;
		void tick().then(() => {
			if (canceled) return;
			const target = Array.from(document.querySelectorAll<HTMLElement>('[data-share-id]')).find(
				(element) => element.dataset.shareId === id,
			);
			target?.scrollIntoView({ block: 'center', behavior: 'smooth' });
		});

		return () => {
			canceled = true;
		};
	});
</script>

<svelte:head>
	<title>Shares · Arkive</title>
</svelte:head>

<div class="mx-auto grid w-full max-w-6xl content-start gap-4 p-4 sm:p-6">
	<section class="overflow-hidden">
		{#if loading}
			<p class="m-0 p-5 text-sm text-(--muted)" role="status">Loading shared links…</p>
		{:else if error}
			<div class="flex flex-wrap items-center justify-between gap-3 p-5">
				<p class="m-0 text-sm text-red-400" role="alert">{error}</p>
				<button
					class="cursor-pointer rounded border border-(--border) px-3 py-2 text-sm"
					type="button"
					onclick={() => void loadShares()}>Retry</button
				>
			</div>
		{:else if shares.length === 0}
			<p class="m-0 p-5 text-sm text-(--muted)">
				No shared links yet. Create one from a file’s context menu.
			</p>
		{:else}
			<div
				class="hidden grid-cols-[minmax(0,1fr)_7rem_9rem_10rem_9rem] items-center gap-4 border-b border-(--border) px-4 py-3 text-xs font-semibold text-(--muted) lg:grid"
				aria-hidden="true"
			>
				<span>Name</span>
				<span>Status</span>
				<span>Permissions</span>
				<span>Expires</span>
				<span class="text-center">Actions</span>
			</div>
			<ul class="m-0 p-0">
				{#each shares as share (share.record.id)}
					{@const busy = actionInProgress?.id === share.record.id}
					<li
						data-share-id={share.record.id}
						class={`grid gap-4 border-b border-(--border) p-4 last:border-b-0 lg:grid-cols-[minmax(0,1fr)_7rem_9rem_10rem_9rem] lg:items-center lg:gap-4 lg:px-4 lg:py-3 ${share.record.isTrashed ? 'opacity-60' : ''} ${focusedShareId === share.record.id ? 'rounded ring-2 ring-(--accent)' : ''}`}
					>
						<div class="flex min-w-0 items-center gap-3">
							<FileTypeIcon
								name={share.entry.name}
								mime={share.entry.type}
								className="size-5 shrink-0"
							/>
							<a
								class="min-w-0 truncate font-medium hover:underline focus-visible:rounded focus-visible:outline-2 focus-visible:outline-(--accent)"
								href={fileLocation(share)}
								title={`Show ${share.entry.name} in ${share.record.isTrashed ? 'Trash' : 'Files'}`}
								>{share.entry.name}</a
							>
						</div>
						<div class="grid min-w-0 gap-2 lg:contents">
							<div
								class="grid min-w-0 grid-cols-[5.5rem_minmax(0,1fr)] items-center gap-3 lg:block"
							>
								<span class="text-xs text-(--muted) lg:hidden">Status</span>
								<span
									class={`inline-flex w-fit rounded-full border px-2 py-0.5 text-xs font-medium ${statusClass(share.record)}`}
								>
									{statusLabel(share.record)}
								</span>
							</div>
							<div
								class="grid min-w-0 grid-cols-[5.5rem_minmax(0,1fr)] items-center gap-3 lg:block"
							>
								<span class="text-xs text-(--muted) lg:hidden">Permissions</span>
								<div class="flex items-center gap-1" role="group" aria-label="Share permissions">
									<Tooltip label={`Preview ${share.record.allowPreview ? 'allowed' : 'disabled'}`}>
										<span
											class="relative grid size-8 place-items-center rounded-md text-(--text)"
											role="img"
											aria-label={`Preview ${share.record.allowPreview ? 'allowed' : 'disabled'}`}
										>
											{#if share.record.allowPreview}
												<Eye class="size-4" aria-hidden="true" />
											{:else}
												<EyeOff class="size-4" aria-hidden="true" />
											{/if}
										</span>
									</Tooltip>
									<Tooltip
										label={`Download ${share.record.allowDownload ? 'allowed' : 'disabled'}`}
									>
										<span
											class="relative grid size-8 place-items-center rounded-md text-(--text)"
											role="img"
											aria-label={`Download ${share.record.allowDownload ? 'allowed' : 'disabled'}`}
										>
											<Download class="size-4" aria-hidden="true" />
											{#if !share.record.allowDownload}
												<span
													class="absolute top-0.5 right-0.5 grid size-3 place-items-center rounded-full bg-(--surface)"
												>
													<X class="size-2.5" strokeWidth={3} aria-hidden="true" />
												</span>
											{/if}
										</span>
									</Tooltip>
									{#if share.record.burnAfterRead}
										<span
											class="inline-flex items-center rounded-full border border-(--border) px-2 py-1 text-xs text-(--muted)"
											>One-time</span
										>
									{/if}
								</div>
							</div>
							<div
								class="grid min-w-0 grid-cols-[5.5rem_minmax(0,1fr)] items-center gap-3 text-sm lg:block"
							>
								<span class="text-xs text-(--muted) lg:hidden">Expires</span>
								<time class="min-w-0" datetime={share.record.expiresAt ?? undefined}>
									{expiryLabel(share.record.expiresAt)}
								</time>
							</div>
						</div>
						<div class="flex justify-end gap-2">
							<button
								class="grid size-10 shrink-0 cursor-pointer place-items-center rounded border border-(--border) hover:bg-(--surface-raised) disabled:cursor-wait disabled:opacity-60"
								type="button"
								disabled={share.record.isTrashed || actionInProgress !== null}
								title={share.record.isTrashed
									? 'Restore the file to use this link'
									: 'Copy share link'}
								aria-label={`${busy && actionInProgress?.kind === 'copy' ? 'Preparing' : 'Copy'} link for ${share.entry.name}${share.record.isTrashed ? ' (file is in Trash)' : ''}`}
								onclick={() => void copyLink(share)}
							>
								<Copy class="size-4" aria-hidden="true" />
							</button>
							<button
								class="grid size-10 shrink-0 cursor-pointer place-items-center rounded border border-(--border) hover:bg-(--surface-raised) disabled:cursor-not-allowed disabled:opacity-50"
								type="button"
								disabled={!canOpen(share.record) || actionInProgress !== null}
								aria-label={`${busy && actionInProgress?.kind === 'open' ? 'Opening' : 'Open'} share for ${share.entry.name}${share.record.isTrashed ? ' (file is in Trash)' : canOpen(share.record) ? '' : ' (active links only)'}`}
								title={share.record.isTrashed
									? 'Restore the file to use this link'
									: 'Open share link'}
								aria-busy={busy && actionInProgress?.kind === 'open'}
								onclick={() => openLink(share)}
							>
								<ExternalLink class="size-4" aria-hidden="true" />
							</button>
							<button
								class="grid size-10 shrink-0 cursor-pointer place-items-center rounded border border-(--border) hover:bg-(--surface-raised) disabled:cursor-not-allowed disabled:opacity-50"
								type="button"
								disabled={share.record.isTrashed || actionInProgress !== null}
								title={share.record.isTrashed
									? 'Restore the file to manage this link'
									: 'Manage share link'}
								aria-label={`Manage link for ${share.entry.name}${share.record.isTrashed ? ' (restore the file first)' : ''}`}
								onclick={() => (selectedShare = share)}
							>
								<Settings class="size-4" aria-hidden="true" />
							</button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</section>
</div>

{#if selectedShare}
	<ShareDialog
		entry={selectedShare.entry}
		vaultId={selectedShare.record.vaultId}
		onClose={closeShare}
	/>
{/if}
