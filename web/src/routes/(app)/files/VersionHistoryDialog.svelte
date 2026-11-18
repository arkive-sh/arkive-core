<script lang="ts">
	import { Download, Eye, History, RotateCcw, Trash2, X } from '@lucide/svelte';
	import Modal from '#lib/components/Modal.svelte';
	import { api } from '#lib/api.ts';
	import { formatBytes } from '#lib/utils/format.ts';
	import { createSearchTokens, decryptSearchFile } from '#lib/vault/search.ts';
	import { encryptFileMetadataForFile, rewrapFileKeyForParent } from '#lib/vault/files.ts';
	import type { FileManagerEntry } from './types';

	type VersionSummary = {
		id: string;
		versionNumber: number;
		actualEncryptedSize: number;
		thumbnailSizeBytes: number;
		createdAt: string;
	};
	type VersionRecord = {
		fileId: string;
		vaultId: string;
		folderId: string | null;
		encryptedMetadata: string;
		encryptedFileKey: string;
	};

	let {
		entry,
		vaultId,
		onClose,
		onPreview,
		onDownload,
	}: {
		entry: FileManagerEntry;
		vaultId: string;
		onClose: () => void;
		onPreview: (entry: FileManagerEntry) => void;
		onDownload: (entry: FileManagerEntry) => void;
	} = $props();
	let currentVersion = $state(1);
	let versions = $state<VersionSummary[]>([]);
	let loading = $state(true);
	let busyVersion = $state('');
	let confirmingDelete = $state('');
	let error = $state('');

	async function loadVersions(): Promise<void> {
		loading = true;
		error = '';
		try {
			const { data } = await api.get<{ currentVersion: number; versions: VersionSummary[] }>(
				`/api/files/${encodeURIComponent(entry.id)}/versions`,
			);
			currentVersion = data.currentVersion;
			versions = data.versions;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load version history.';
		} finally {
			loading = false;
		}
	}

	async function getVersion(version: VersionSummary): Promise<{
		record: VersionRecord;
		metadata: Record<string, unknown> & { name?: string; mime?: string; size?: number };
	}> {
		const { data: record } = await api.get<VersionRecord>(
			`/api/files/${encodeURIComponent(entry.id)}/versions/${encodeURIComponent(version.id)}/record`,
		);
		const decrypted = await decryptSearchFile({
			id: entry.id,
			folderId: record.folderId,
			vaultId: record.vaultId,
			encryptedMetadata: record.encryptedMetadata,
			encryptedFileKey: record.encryptedFileKey,
			score: 0,
			url: '',
		});

		return { record, metadata: decrypted.metadata };
	}

	function preview(version: VersionSummary): void {
		error = '';
		onPreview({
			...entry,
			versionId: version.id,
			versionNumber: version.versionNumber,
			updatedAt: version.createdAt,
		});
		onClose();
	}

	async function restore(version: VersionSummary): Promise<void> {
		busyVersion = version.id;
		error = '';
		try {
			const { record, metadata } = await getVersion(version);
			const { data: currentRecord } = await api.get<VersionRecord>(
				`/api/files/${encodeURIComponent(entry.id)}/record`,
			);
			const current = await decryptSearchFile({
				id: entry.id,
				folderId: currentRecord.folderId,
				vaultId: currentRecord.vaultId,
				encryptedMetadata: currentRecord.encryptedMetadata,
				encryptedFileKey: currentRecord.encryptedFileKey,
				score: 0,
				url: '',
			});
			const parentFolderId = currentRecord.folderId;
			const encryptedFileKey = await rewrapFileKeyForParent(
				{
					vaultId: record.vaultId,
					fileId: entry.id,
					folderId: record.folderId,
					encryptedFileKey: record.encryptedFileKey,
				},
				parentFolderId,
			);
			const restoredMetadata = { ...metadata, name: current.metadata.name || entry.name };
			const encryptedMetadata = await encryptFileMetadataForFile(
				restoredMetadata,
				vaultId,
				entry.id,
				parentFolderId,
				encryptedFileKey.encryptedFileKey,
			);
			const searchTokens = (await createSearchTokens(restoredMetadata.name, vaultId, 128)).map(
				(token) => ({
					token,
					field: 'name' as const,
					weight: 10 as const,
				}),
			);
			await api.post(
				`/api/files/${encodeURIComponent(entry.id)}/versions/${encodeURIComponent(version.id)}/restore`,
				{
					folderId: parentFolderId,
					encryptedMetadata: encryptedMetadata.encryptedMetadata,
					encryptedFileKey: encryptedFileKey.encryptedFileKey,
					searchTokens,
				},
			);
			window.dispatchEvent(new Event('arkive:upload-complete'));
			await loadVersions();
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not restore this version.';
		} finally {
			busyVersion = '';
		}
	}

	async function deleteVersion(version: VersionSummary): Promise<void> {
		if (confirmingDelete !== version.id) {
			confirmingDelete = version.id;

			return;
		}
		busyVersion = version.id;
		error = '';
		try {
			await api.delete(
				`/api/files/${encodeURIComponent(entry.id)}/versions/${encodeURIComponent(version.id)}`,
			);
			versions = versions.filter((item) => item.id !== version.id);
			confirmingDelete = '';
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not delete this version.';
		} finally {
			busyVersion = '';
		}
	}

	$effect(() => {
		void entry.id;
		void loadVersions();
	});
</script>

<Modal
	open
	label={`Version history for ${entry.name}`}
	{onClose}
	dialogClass="w-[min(1040px,100%)]! !max-h-[88vh] !rounded-xl max-[640px]:!max-h-[92vh] max-[640px]:!rounded-t-xl"
	bodyClass="!gap-6 !px-8 !py-7 max-[640px]:!gap-5 max-[640px]:!px-4 max-[640px]:!py-5"
>
	{#snippet header()}
		<header
			class="flex items-center justify-between gap-5 border-b border-(--border) px-8 py-6 max-[640px]:px-4 max-[640px]:py-4"
		>
			<div class="flex min-w-0 items-center gap-4">
				<span
					class="grid size-12 shrink-0 place-items-center rounded-lg bg-(--surface-raised) text-(--accent)"
				>
					<History class="size-6" aria-hidden="true" />
				</span>
				<div class="min-w-0">
					<h2 class="m-0 text-xl leading-7 font-semibold">Version history</h2>
					<p class="m-0 truncate text-sm text-(--muted)" title={entry.name}>{entry.name}</p>
				</div>
			</div>
			<button
				class="grid size-10 shrink-0 cursor-pointer place-items-center rounded-md border border-(--border) text-(--muted) transition-colors hover:bg-(--surface-raised) hover:text-(--text) focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--accent)"
				type="button"
				aria-label="Close version history"
				onclick={onClose}
			>
				<X class="size-5" aria-hidden="true" />
			</button>
		</header>
	{/snippet}
	{#if error}
		<p
			class="m-0 rounded-md border border-red-500/30 bg-red-500/8 px-4 py-3 text-sm text-red-300"
			role="alert"
		>
			{error}
		</p>
	{/if}
	<div
		class="flex flex-wrap items-center justify-between gap-x-6 gap-y-2 border-l-2 border-(--accent) bg-(--surface-raised) px-5 py-4"
	>
		<p class="m-0 text-base font-medium">
			Current version <span class="ml-2 text-(--accent)">v{currentVersion}</span>
		</p>
		<p class="m-0 text-sm text-(--muted)">Restored versions stay in your history.</p>
	</div>
	<div class="grid gap-3">
		<div class="flex items-baseline justify-between gap-4">
			<h3 class="m-0 text-base font-semibold">Previous versions</h3>
			{#if !loading && versions.length > 0}
				<span class="text-sm text-(--muted)">{versions.length} saved</span>
			{/if}
		</div>
		{#if loading}
			<div
				class="divide-y divide-(--border) border-y border-(--border)"
				aria-label="Loading version history"
				aria-busy="true"
			>
				{#each [1, 2] as row (row)}
					<div class="flex items-center justify-between gap-6 py-5" aria-hidden="true">
						<div class="grid min-w-0 flex-1 gap-3">
							<div class="h-5 w-36 animate-pulse rounded bg-(--surface-raised)"></div>
							<div class="h-4 w-56 max-w-full animate-pulse rounded bg-(--surface-raised)"></div>
						</div>
						<div
							class="hidden h-10 w-96 max-w-[48%] animate-pulse rounded bg-(--surface-raised) sm:block"
						></div>
					</div>
				{/each}
			</div>
		{:else if versions.length === 0}
			<div class="border-y border-(--border) py-10 text-center">
				<p class="m-0 font-medium">No previous versions</p>
				<p class="mt-2 mb-0 text-sm text-(--muted)">
					Upload another file with this name in the same folder to create one.
				</p>
			</div>
		{:else}
			<ul class="m-0 divide-y divide-(--border) border-y border-(--border) p-0">
				{#each versions as version (version.id)}
					<li
						class="grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-6 gap-y-4 py-5 max-[820px]:grid-cols-1 max-[820px]:py-4"
					>
						<div class="flex min-w-0 items-center gap-4">
							<span
								class="grid size-12 shrink-0 place-items-center rounded-md bg-(--surface-raised) text-lg font-semibold text-(--muted)"
							>
								v{version.versionNumber}
							</span>
							<div class="min-w-0">
								<p class="m-0 font-medium">{new Date(version.createdAt).toLocaleString()}</p>
								<p class="m-0 mt-1 text-sm text-(--muted)">
									{formatBytes(version.actualEncryptedSize + version.thumbnailSizeBytes)} stored
								</p>
							</div>
						</div>
						<div class="flex flex-wrap items-center gap-2 max-[820px]:pl-16">
							<button
								class="inline-flex min-h-10 cursor-pointer items-center gap-2 rounded-md border border-(--border) px-3.5 text-sm font-medium transition-colors hover:bg-(--surface-raised) focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--accent) disabled:cursor-wait disabled:opacity-50"
								type="button"
								disabled={!!busyVersion}
								onclick={() => preview(version)}
							>
								<Eye class="size-4" aria-hidden="true" /> Preview
							</button>
							<button
								class="inline-flex min-h-10 cursor-pointer items-center gap-2 rounded-md border border-(--border) px-3.5 text-sm font-medium transition-colors hover:bg-(--surface-raised) focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--accent) disabled:cursor-wait disabled:opacity-50"
								type="button"
								disabled={!!busyVersion}
								onclick={() =>
									onDownload({
										...entry,
										versionId: version.id,
										versionNumber: version.versionNumber,
										size: version.actualEncryptedSize,
										updatedAt: version.createdAt,
									})}
							>
								<Download class="size-4" aria-hidden="true" /> Download
							</button>
							<button
								class="inline-flex min-h-10 cursor-pointer items-center gap-2 rounded-md border border-(--border) px-3.5 text-sm font-medium transition-colors hover:bg-(--surface-raised) focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-(--accent) disabled:cursor-wait disabled:opacity-50"
								type="button"
								disabled={!!busyVersion}
								onclick={() => void restore(version)}
							>
								<RotateCcw class="size-4" aria-hidden="true" /> Restore
							</button>
							<button
								class="inline-flex min-h-10 cursor-pointer items-center gap-2 rounded-md border border-red-500/40 px-3.5 text-sm font-medium text-red-400 transition-colors hover:border-red-500/60 hover:bg-red-500/10 focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-red-400 disabled:cursor-wait disabled:opacity-50"
								type="button"
								disabled={!!busyVersion}
								onclick={() => void deleteVersion(version)}
							>
								<Trash2 class="size-4" aria-hidden="true" />
								{confirmingDelete === version.id ? 'Confirm delete' : 'Delete'}
							</button>
						</div>
					</li>
				{/each}
			</ul>
		{/if}
	</div>
</Modal>
