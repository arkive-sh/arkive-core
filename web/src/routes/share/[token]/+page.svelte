<script lang="ts">
	import { page } from '$app/state';
	import { onDestroy, onMount } from 'svelte';
	import { SvelteSet } from 'svelte/reactivity';
	import {
		Database,
		Download,
		Eye,
		EyeOff,
		Folder,
		LayoutGrid,
		LayoutList,
		LockKeyhole,
	} from '@lucide/svelte';
	import axios from 'axios';
	import { api } from '#lib/api.ts';
	import FileTypeIcon from '#lib/components/FileTypeIcon.svelte';
	import { streamDownload } from '#lib/downloads/stream.ts';
	import { useTransferManager } from '#lib/transfers/manager.ts';
	import FileEntryContextMenu from '../../(app)/files/FileEntryContextMenu.svelte';
	import FilesGridView from '../../(app)/files/FilesGridView.svelte';
	import FilesListView from '../../(app)/files/FilesListView.svelte';
	import { formatBytes } from '#lib/utils/format.ts';
	import { isAudioMimeType } from '#lib/utils/media.ts';
	import { isImageFile } from '#lib/utils/image.ts';
	import { decodeJSON, fromBase64 } from '#lib/vault/bytes.ts';
	import {
		closePublicShareContext,
		decryptPublicShareChunk,
		openPublicShareFileMetadata,
		openPublicShareFolder,
		openPublicShareContext,
	} from '#lib/vault/shares.ts';
	import {
		DecryptedFileReader,
		type EncryptedFileRecord,
		type FileReaderSource,
	} from '#lib/vault/read-file.ts';
	import type { PublicShareCryptoRecord, VaultMetadata } from '#lib/vault/types.ts';
	import { isPdfFile, type FileManagerEntry, type FilesView } from '../../(app)/files/types';
	import EncryptedMediaPlayer from '../../(app)/files/EncryptedMediaPlayer.svelte';
	import ImageViewer from '../../(app)/files/ImageViewer.svelte';
	import MediaViewer from '../../(app)/files/MediaViewer.svelte';
	import PdfViewer from '../../(app)/files/PdfViewer.svelte';

	type PublicShareRecord = {
		itemType: 'file' | 'folder';
		shareId: string;
		token: string;
		fileId?: string;
		folderId?: string;
		vaultId: string;
		encryptedFolderName?: string;
		encryptedFolderMetadata?: string;
		encryptionVersion: number;
		chunkSize: number;
		totalChunks: number;
		encryptedHash: string;
		encryptedMetadata: string;
		encryptedManifest: string;
		encryptedFileKeyForShare: string;
		sourceUrl: string;
		allowPreview: boolean;
		allowDownload: boolean;
	};
	type PublicFolderEntry = {
		id: string;
		kind: 'folder' | 'file';
		name: string;
		type: string;
		size: number | null;
		updatedAt: string;
		vaultId: string;
		encryptedName?: string;
		encryptedMetadata?: string;
		encryptedFileKeyForShare?: string;
	};

	type FilePickerWindow = Window & {
		showSaveFilePicker?: (options?: { suggestedName?: string }) => Promise<FileSystemFileHandle>;
	};

	let shareKey: Uint8Array | null = null;
	const token = $derived(page.params.token ?? '');
	const transfers = useTransferManager();
	let fileReader = $state<DecryptedFileReader | undefined>();
	let openedRecord = $state<EncryptedFileRecord | null>(null);
	let metadata = $state<(VaultMetadata & { name?: string; mime?: string; size?: number }) | null>(
		null,
	);
	let permissions = $state({ preview: false, download: false });
	let folderShare = $state<PublicShareRecord | null>(null);
	let folderPath = $state<Array<{ id: string; name: string }>>([]);
	let folderEntries = $state<PublicFolderEntry[]>([]);
	const noSelection = new SvelteSet<string>();
	let folderView = $state<FilesView>('list');
	let folderCursor = $state<string | null>(null);
	let folderLoading = $state(false);
	let previewOpen = $state(false);
	let imagePreviewRequest = $state<{ fileId: string } | null>(null);
	let loading = $state(true);
	let startingDownload = $state(false);
	let contextMenuEntry = $state<FileManagerEntry | null>(null);
	let contextMenuPosition = $state({ x: 0, y: 0 });
	let contextMenuOpen = $derived(contextMenuEntry !== null);
	let passwordRequired = $state(false);
	let password = $state('');
	let error = $state('');

	function closePreview(): void {
		previewOpen = false;
		imagePreviewRequest = null;
		if (folderShare) {
			void fileReader?.dispose();
			fileReader = undefined;
			openedRecord = null;
			metadata = null;
		}
	}

	function createShareReader(
		share: PublicShareRecord,
		secret: Uint8Array,
		signal?: AbortSignal,
	): DecryptedFileReader {
		const cryptoRecord: PublicShareCryptoRecord = {
			token: share.token,
			fileId: share.fileId!,
			vaultId: share.vaultId,
			encryptedFileKeyForShare: share.encryptedFileKeyForShare,
			encryptedMetadata: share.encryptedMetadata,
			encryptedManifest: share.encryptedManifest,
		};
		const record: EncryptedFileRecord = {
			fileId: share.fileId!,
			vaultId: share.vaultId,
			folderId: null,
			encryptedFileKey: '',
			encryptedMetadata: share.encryptedMetadata,
			encryptedManifest: share.encryptedManifest,
			encryptionVersion: share.encryptionVersion,
			chunkSize: share.chunkSize,
			totalChunks: share.totalChunks,
			encryptedHash: share.encryptedHash,
			sourceUrl: share.sourceUrl,
		};
		const readerSecret = secret.slice();
		const source: FileReaderSource = {
			loadRecord: async () => record,
			openContext: (contextId) =>
				openPublicShareContext(contextId, cryptoRecord, readerSecret.slice()),
			decryptChunk: (contextId, chunk, aad, expectedHash) =>
				decryptPublicShareChunk(contextId, chunk, aad, expectedHash),
			closeContext: closePublicShareContext,
			dispose: () => readerSecret.fill(0),
		};

		return new DecryptedFileReader(share.fileId!, signal, source);
	}

	async function decryptFolderName(
		folderId: string,
		encryptedName: string,
		encryptedMetadata: string,
	): Promise<string> {
		if (!shareKey || !folderShare) throw new Error('Share key is unavailable.');
		const decrypted = await openPublicShareFolder(
			folderId,
			token,
			encryptedName,
			encryptedMetadata,
			shareKey.slice(),
		);
		const nameMetadata = decodeJSON<VaultMetadata>(decrypted.nameBytes);
		decodeJSON<VaultMetadata>(decrypted.metadataBytes);

		return typeof nameMetadata.name === 'string' ? nameMetadata.name : 'Shared folder';
	}

	async function loadFolderEntries(folderId: string, append = false): Promise<void> {
		if (!folderShare) return;
		folderLoading = true;
		error = '';
		if (!append) {
			folderEntries = [];
			folderCursor = null;
		}
		try {
			const { data } = await api.get<{
				folders: Array<{
					id: string;
					vaultId: string;
					parentFolderId: string | null;
					encryptedName: string;
					encryptedMetadata: string;
					updatedAt: string;
				}>;
				files: Array<{
					id: string;
					vaultId: string;
					folderId: string;
					encryptedMetadata: string;
					encryptedFileKeyForShare: string;
					updatedAt: string;
				}>;
				nextCursor: string;
			}>(
				`/api/public/shares/${encodeURIComponent(token)}/folders/${encodeURIComponent(folderId)}/entries`,
				{
					params: { pageSize: 50, cursor: append ? (folderCursor ?? undefined) : undefined },
				},
			);
			const folders = await Promise.all(
				data.folders.map(async (folder): Promise<PublicFolderEntry> => ({
					id: folder.id,
					kind: 'folder',
					name: await decryptFolderName(folder.id, folder.encryptedName, folder.encryptedMetadata),
					type: 'Folder',
					size: null,
					updatedAt: folder.updatedAt,
					vaultId: folder.vaultId,
					encryptedName: folder.encryptedName,
					encryptedMetadata: folder.encryptedMetadata,
				})),
			);
			const files = await Promise.all(
				data.files.map(async (file): Promise<PublicFolderEntry> => {
					if (!shareKey) throw new Error('Share key is unavailable.');
					const decrypted = await openPublicShareFileMetadata(
						{
							token,
							fileId: file.id,
							vaultId: file.vaultId,
							encryptedFileKeyForShare: file.encryptedFileKeyForShare,
							encryptedMetadata: file.encryptedMetadata,
						},
						shareKey.slice(),
					);
					const metadata = decodeJSON<VaultMetadata>(decrypted.metadataBytes);

					return {
						id: file.id,
						kind: 'file',
						name: typeof metadata.name === 'string' ? metadata.name : 'Shared file',
						type: typeof metadata.mime === 'string' ? metadata.mime : 'application/octet-stream',
						size: typeof metadata.size === 'number' ? metadata.size : null,
						updatedAt: file.updatedAt,
						vaultId: file.vaultId,
						encryptedMetadata: file.encryptedMetadata,
						encryptedFileKeyForShare: file.encryptedFileKeyForShare,
					};
				}),
			);
			const nextEntries = [...folders, ...files].sort((a, b) => a.name.localeCompare(b.name));
			folderEntries = append ? [...folderEntries, ...nextEntries] : nextEntries;
			folderCursor = data.nextCursor || null;
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not load this folder.';

			throw cause;
		} finally {
			folderLoading = false;
		}
	}

	async function openFolder(entry: PublicFolderEntry): Promise<void> {
		try {
			await loadFolderEntries(entry.id);
			folderPath = [...folderPath, { id: entry.id, name: entry.name }];
		} catch {
			// The error is shown by the folder view.
		}
	}

	function openFolderById(folderId: string): void {
		const entry = folderEntries.find(
			(candidate) => candidate.kind === 'folder' && candidate.id === folderId,
		);

		if (entry) void openFolder(entry);
	}

	function showContextMenu(entry: FileManagerEntry | null, x: number, y: number): void {
		contextMenuEntry = entry;
		contextMenuPosition = { x, y };
	}

	async function openFolderFile(entry: FileManagerEntry): Promise<void> {
		loading = true;
		error = '';
		try {
			const { data: share } = await api.get<PublicShareRecord>(
				`/api/public/shares/${encodeURIComponent(token)}/files/${encodeURIComponent(entry.id)}`,
			);
			permissions = { preview: share.allowPreview, download: share.allowDownload };
			if (folderShare && !share.allowPreview)
				throw new Error('The owner has disabled previews for this link.');
			const reader = createShareReader(share, shareKey!);
			fileReader = reader;
			const loaded = await reader.load();

			if (folderShare && !supportsPreview(loaded.metadata)) {
				await reader.dispose();
				fileReader = undefined;

				throw new Error(
					permissions.download
						? 'This file type cannot be previewed. Right-click to download.'
						: 'This file type cannot be previewed.',
				);
			}
			openedRecord = loaded.record;
			metadata = loaded.metadata;
			if (folderShare && permissions.preview) {
				previewOpen = true;
				imagePreviewRequest = isImageFile(entry.name, entry.type) ? { fileId: entry.id } : null;
			} else if (permissions.preview && supportsPreview(loaded.metadata)) {
				if (isImageFile(entry.name, entry.type) || isPdfFile(entry)) {
					previewOpen = true;
					imagePreviewRequest = isImageFile(entry.name, entry.type) ? { fileId: entry.id } : null;
				}
			}
		} catch (cause) {
			await fileReader?.dispose();
			fileReader = undefined;
			openedRecord = null;
			metadata = null;
			error = cause instanceof Error ? cause.message : 'Could not open this file.';
		} finally {
			loading = false;
		}
	}

	async function navigateToBreadcrumb(index: number): Promise<void> {
		const destination = folderPath[index];

		if (!destination || index === folderPath.length - 1) return;
		try {
			await loadFolderEntries(destination.id);
			folderPath = folderPath.slice(0, index + 1);
		} catch {
			// The error is shown by the folder view.
		}
	}

	async function getCurrentShareFile(
		fileId: string,
		signal?: AbortSignal,
	): Promise<PublicShareRecord> {
		const path = folderShare
			? `/api/public/shares/${encodeURIComponent(token)}/files/${encodeURIComponent(fileId)}`
			: `/api/public/shares/${encodeURIComponent(token)}`;
		const { data } = await api.get<PublicShareRecord>(path, { signal });

		return data;
	}

	async function openShare(): Promise<void> {
		loading = true;
		passwordRequired = false;
		error = '';
		closePreview();
		await fileReader?.dispose();
		fileReader = undefined;
		openedRecord = null;
		metadata = null;
		folderShare = null;
		folderPath = [];
		folderEntries = [];
		folderCursor = null;

		try {
			const { data: share } = await api.get<PublicShareRecord>(
				`/api/public/shares/${encodeURIComponent(token)}`,
			);

			if (!shareKey)
				throw new Error(
					'Your link is missing its decryption key. Ask the sender for the complete link.',
				);
			permissions = { preview: share.allowPreview, download: share.allowDownload };
			if (share.itemType === 'folder') {
				if (!share.folderId || !share.encryptedFolderName || !share.encryptedFolderMetadata)
					throw new Error('The shared folder is unavailable.');
				folderShare = share;
				const name = await decryptFolderName(
					share.folderId,
					share.encryptedFolderName,
					share.encryptedFolderMetadata,
				);
				folderPath = [{ id: share.folderId, name }];
				await loadFolderEntries(share.folderId);

				return;
			}
			const reader = createShareReader(share, shareKey);
			fileReader = reader;
			const loaded = await reader.load();
			openedRecord = loaded.record;
			metadata = loaded.metadata;
		} catch (cause) {
			if (axios.isAxiosError(cause) && cause.response?.status === 403) {
				passwordRequired = true;
			} else {
				error = cause instanceof Error ? cause.message : 'Could not open this share.';
			}
			await fileReader?.dispose();
			fileReader = undefined;
		} finally {
			loading = false;
		}
	}

	function supportsPreview(file: VaultMetadata & { name?: string; mime?: string }): boolean {
		const mime = typeof file.mime === 'string' ? file.mime : '';

		return (
			isImageFile(file.name || '', mime) ||
			isAudioMimeType(mime) ||
			mime.startsWith('video/') ||
			mime.toLowerCase() === 'application/pdf' ||
			file.name?.toLowerCase().endsWith('.pdf') === true
		);
	}

	function openFilePreview(): void {
		if (!shareEntry || !metadata || !permissions.preview) return;
		previewOpen = true;
		imagePreviewRequest = isImageFile(shareEntry.name, shareEntry.type)
			? { fileId: shareEntry.id }
			: null;
	}

	function safeDownloadName(name: string): string {
		return name.replace(/[<>:"/\\|?*\p{Cc}]/gu, '_').replace(/[. ]+$/g, '') || 'download';
	}

	function showDownloadError(cause: unknown): void {
		if (cause instanceof DOMException && cause.name === 'AbortError') return;
		error = cause instanceof Error ? cause.message : 'Could not start the download.';
	}

	function downloadSharedFile(entry: FileManagerEntry | null = shareEntry): void {
		if (!entry || entry.kind !== 'file' || !permissions.download || !shareKey || startingDownload)
			return;
		const fileId = entry.id;
		const picker = window as FilePickerWindow;

		if (!picker.showSaveFilePicker) {
			error = 'This browser cannot save streamed files directly to disk.';

			return;
		}

		startingDownload = true;
		error = '';
		try {
			void picker
				.showSaveFilePicker({ suggestedName: safeDownloadName(entry.name || 'shared-file') })
				.then((target) => {
					if (!shareKey) return;
					const jobSecret = shareKey.slice();
					const fileName = target.name || safeDownloadName(entry.name || 'shared-file');

					transfers.enqueueDownload(
						fileName,
						typeof entry.size === 'number' ? entry.size : 0,
						async (signal, onProgress, concurrency) => {
							const currentShare = await getCurrentShareFile(fileId, signal);

							if (!currentShare.allowDownload)
								throw new Error('The owner has disabled downloads for this share.');

							const reader = createShareReader(currentShare, jobSecret, signal);

							try {
								const file = await reader.load();
								await streamDownload({
									reader,
									record: file.record,
									metadata: file.metadata,
									purpose: 'download',
									concurrency,
									target,
									signal,
									onProgress,
								});
							} finally {
								await reader.dispose();
							}
						},
						() => jobSecret.fill(0),
					);
				})
				.catch(showDownloadError)
				.finally(() => (startingDownload = false));
		} catch (cause) {
			startingDownload = false;
			showDownloadError(cause);
		}
	}

	async function unlockShare(): Promise<void> {
		if (!password) return;
		loading = true;
		error = '';
		try {
			await api.post(`/api/public/shares/${encodeURIComponent(token)}/unlock`, { password });
			password = '';
			await openShare();
		} catch (cause) {
			password = '';
			loading = false;
			error = cause instanceof Error ? cause.message : 'Could not unlock this share.';
		}
	}

	function submitPassword(event: SubmitEvent): void {
		event.preventDefault();
		void unlockShare();
	}

	let shareEntry = $derived.by((): FileManagerEntry | null => {
		if (!openedRecord || !metadata) return null;

		return {
			id: openedRecord.fileId,
			kind: 'file',
			name: metadata.name || 'Shared file',
			type: typeof metadata.mime === 'string' ? metadata.mime : 'application/octet-stream',
			size: typeof metadata.size === 'number' ? metadata.size : null,
			updatedAt: '',
		};
	});

	onMount(() => {
		const value = new URLSearchParams(window.location.hash.slice(1)).get('key');

		if (!value) {
			loading = false;
			error = 'Your link is missing its decryption key. Ask the sender for the complete link.';

			return;
		}
		try {
			shareKey = fromBase64(value);
			if (shareKey.byteLength !== 32) throw new Error('Invalid share key.');
			void openShare();
		} catch {
			shareKey?.fill(0);
			shareKey = null;
			loading = false;
			error = 'This share link has an invalid decryption key.';
		}
	});

	onDestroy(() => {
		shareKey?.fill(0);
		shareKey = null;
		closePreview();
		void fileReader?.dispose();
	});
</script>

<svelte:head>
	<title>{folderShare ? 'Shared folder' : 'Shared file'} · Arkive</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<main
	class="box-border min-h-dvh overflow-x-hidden bg-[radial-gradient(560px_280px_at_14%_0%,rgb(56_189_248_/_10%),transparent_68%),radial-gradient(640px_320px_at_86%_12%,rgb(18_26_34_/_70%),transparent_72%),#0b0e11] px-6 py-2 text-[#dee3e8] max-[920px]:px-3 max-[640px]:p-1.5"
>
	<div class="mx-auto flex min-h-dvh w-full max-w-[1296px] flex-col">
		{#if loading}
			<section
				class="grid min-h-80 place-items-center rounded-lg border border-[#30363d] bg-[#161b22] p-8 text-center text-[#87929a]"
				role="status"
			>
				Opening shared file…
			</section>
		{:else if passwordRequired}
			<section
				class="grid min-h-80 place-items-center rounded-lg border border-[#30363d] bg-[#161b22] p-8 text-center text-[#87929a]"
			>
				<form
					class="grid w-[min(100%,380px)] justify-items-stretch gap-3 text-left"
					onsubmit={submitPassword}
				>
					<LockKeyhole class="size-8 text-sky-400" aria-hidden="true" />
					<h1 class="m-0 text-2xl text-[#e5eaef]">This link is password protected</h1>
					<p class="m-0 text-[#87929a]">
						Enter the password provided by the sender to open this file.
					</p>
					<label for="share-password">Share password</label>
					<input
						id="share-password"
						class="rounded border border-(--border) bg-(--bg) px-3 py-2.5"
						type="password"
						autocomplete="current-password"
						bind:value={password}
					/>
					{#if error}<p class="m-0 leading-[1.5] text-red-400" role="alert">{error}</p>{/if}
					<button
						class="inline-flex min-h-12 cursor-pointer items-center justify-center gap-3 rounded-[4px] border-0 bg-[linear-gradient(180deg,#4ab8f0,#3aa9e3)] px-6 text-[13px] font-extrabold tracking-[0.08em] text-[#052235] uppercase hover:brightness-[1.06] disabled:cursor-wait disabled:opacity-[0.65] max-[640px]:min-h-[42px] max-[640px]:px-[14px] max-[640px]:text-[11px]"
						type="submit"
						disabled={!password}>Open share</button
					>
				</form>
			</section>
		{:else if error && !folderShare}
			<section
				class="grid min-h-80 place-items-center rounded-lg border border-[#30363d] bg-[#161b22] p-8 text-center text-[#87929a]"
				role="alert"
			>
				{error}
			</section>
		{:else if folderShare}
			<section class="flex min-h-0 flex-1 flex-col">
				<nav
					class="flex min-w-0 items-center gap-3 border-b border-[#30363d] py-3 text-sm"
					aria-label="Folder path"
				>
					<div class="flex min-w-0 flex-1 items-center gap-2 overflow-x-auto">
						<Folder class="size-5 shrink-0 text-sky-400" aria-hidden="true" />
						{#each folderPath as crumb, index (crumb.id)}
							{#if index > 0}<span class="text-[#6f7983]" aria-hidden="true">/</span>{/if}
							{#if index < folderPath.length - 1}
								<button
									class="max-w-52 shrink-0 cursor-pointer truncate border-0 bg-transparent p-0 text-[#aeb8c2] hover:text-white"
									type="button"
									onclick={() => void navigateToBreadcrumb(index)}>{crumb.name}</button
								>
							{:else}
								<span class="max-w-52 shrink-0 truncate text-[#e5eaef]" aria-current="page"
									>{crumb.name}</span
								>
							{/if}
						{/each}
					</div>
					<span class="shrink-0 text-[#87929a] max-[520px]:hidden">Read-only</span>
					<div
						class="inline-flex shrink-0 items-center rounded-md border border-[#30363d] bg-[#161b22] p-1"
						role="group"
						aria-label="Folder view"
					>
						<button
							class={`inline-flex size-9 cursor-pointer items-center justify-center rounded transition-colors ${folderView === 'list' ? 'bg-[#30363d] text-[#e5eaef]' : 'text-[#87929a] hover:text-white'}`}
							type="button"
							aria-label="List view"
							aria-pressed={folderView === 'list'}
							onclick={() => (folderView = 'list')}
						>
							<LayoutList class="size-4.5" aria-hidden="true" />
						</button>
						<button
							class={`inline-flex size-9 cursor-pointer items-center justify-center rounded transition-colors ${folderView === 'grid' ? 'bg-[#30363d] text-[#e5eaef]' : 'text-[#87929a] hover:text-white'}`}
							type="button"
							aria-label="Grid view"
							aria-pressed={folderView === 'grid'}
							onclick={() => (folderView = 'grid')}
						>
							<LayoutGrid class="size-4.5" aria-hidden="true" />
						</button>
					</div>
				</nav>
				{#if error}<p class="m-0 border-b border-[#30363d] py-3 text-red-400" role="alert">
						{error}
					</p>{/if}
				{#if folderEntries.length === 0 && folderLoading}
					<p class="py-8 text-center text-[#87929a]" role="status">Loading folder…</p>
				{:else if folderView === 'grid'}
					<FilesGridView
						rows={folderEntries}
						focusFileId={null}
						selectedIds={noSelection}
						selectionEnabled={false}
						previewEnabled={permissions.preview}
						tileSize={200}
						showType={false}
						showSize={true}
						showDimensions={false}
						creatingFolder={false}
						onCreateFolder={() => {}}
						onCancelNewFolder={() => {}}
						onToggleSelection={() => {}}
						onOpenFolder={openFolderById}
						onShowContextMenu={showContextMenu}
						onPreviewImage={(entry) => void openFolderFile(entry)}
						onPreviewMedia={(entry) => void openFolderFile(entry)}
						onPreviewPdf={(entry) => void openFolderFile(entry)}
						onPreviewText={(entry) => void openFolderFile(entry)}
					/>
				{:else}
					<FilesListView
						rows={folderEntries}
						focusFileId={null}
						selectedIds={noSelection}
						selectionEnabled={false}
						previewEnabled={permissions.preview}
						selectedCount={0}
						loadedCount={folderEntries.length}
						showDimensions={false}
						creatingFolder={false}
						onCreateFolder={() => {}}
						onCancelNewFolder={() => {}}
						onToggleSelection={() => {}}
						onToggleAll={() => {}}
						onOpenFolder={openFolderById}
						onShowContextMenu={showContextMenu}
						onPreviewImage={(entry) => void openFolderFile(entry)}
						onPreviewMedia={(entry) => void openFolderFile(entry)}
						onPreviewPdf={(entry) => void openFolderFile(entry)}
						onPreviewText={(entry) => void openFolderFile(entry)}
					/>
				{/if}
				{#if folderCursor}
					<div class="flex justify-center border-t border-[#30363d] py-4">
						<button
							class="cursor-pointer rounded border border-[#30363d] bg-transparent px-4 py-2 text-[#dee3e8] disabled:opacity-50"
							type="button"
							disabled={folderLoading}
							onclick={() => void loadFolderEntries(folderPath.at(-1)!.id, true)}
							>{folderLoading ? 'Loading…' : 'Load more'}</button
						>
					</div>
				{/if}
			</section>
		{:else if metadata && shareEntry}
			<section
				class="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)] grid-rows-[auto_minmax(0,1fr)] overflow-hidden rounded-lg border border-[#30363d] bg-[#161b22] shadow-[0_28px_90px_rgb(0_0_0_/_42%)]"
			>
				<aside
					class="grid min-w-0 gap-3 px-6 py-4 max-[920px]:px-[18px] max-[920px]:py-[14px] max-[640px]:gap-2.5 max-[640px]:px-3 max-[640px]:py-2.5"
				>
					<div class="flex min-w-0 items-center justify-between gap-4">
						<div class="flex min-w-0 items-center gap-3">
							<FileTypeIcon
								name={shareEntry.name}
								mime={shareEntry.type}
								className="size-6 shrink-0"
							/>
							<h1
								class="m-0 min-w-0 overflow-hidden text-ellipsis text-[clamp(1.1rem,1.5vw,1.4rem)] leading-[1.2] font-medium tracking-[-0.02em] whitespace-nowrap text-[#e5eaef]"
							>
								{shareEntry.name}
							</h1>
						</div>
						<div
							class="inline-flex flex-[0_0_auto] items-center gap-1.5 text-[13px] text-[#87929a]"
						>
							<Database class="size-4 shrink-0" aria-hidden="true" />
							<span>{formatBytes(shareEntry.size)}</span>
						</div>
					</div>
				</aside>

				<div
					class="relative grid min-h-0 min-w-0 place-items-center overflow-hidden border-t border-[#30363d] bg-[radial-gradient(circle_at_top,rgb(78_140_173_/_12%),transparent_46%),#0d1217]"
				>
					<div class="grid w-full justify-items-center gap-5 p-6">
						{#if permissions.preview && supportsPreview(metadata)}
							{#if isAudioMimeType(shareEntry.type) || shareEntry.type.startsWith('video/')}
								<div class="w-[min(100%,1120px)]">
									<EncryptedMediaPlayer
										fileId={shareEntry.id}
										fileName={shareEntry.name}
										mimeType={shareEntry.type}
										reader={fileReader}
									/>
								</div>
							{:else}
								<div class="grid justify-items-center gap-3 p-8 text-center text-[#87929a]">
									<strong class="text-lg text-[#dee3e8]">Preview is ready</strong>
									<p class="m-0 leading-[1.55] text-[#87929a]">
										Open this file in Arkive’s secure viewer.
									</p>
								</div>
							{/if}
						{:else}
							<div class="grid justify-items-center gap-3 p-8 text-center text-[#87929a]">
								<EyeOff class="size-8 text-(--muted)" aria-hidden="true" />
								<strong class="text-lg text-[#dee3e8]">Preview unavailable</strong>
								<p class="m-0 leading-[1.55] text-[#87929a]">
									{permissions.preview
										? 'This file type cannot be previewed in the browser.'
										: 'The owner has disabled previews for this link.'}
								</p>
							</div>
						{/if}

						{#if !folderShare}
							<div class="flex flex-wrap justify-center gap-3 max-[640px]:gap-2">
								{#if permissions.preview && (isImageFile(shareEntry.name, shareEntry.type) || isPdfFile(shareEntry))}
									<button
										class="inline-flex min-h-12 cursor-pointer items-center justify-center gap-3 rounded-[4px] border-0 bg-[linear-gradient(180deg,#4ab8f0,#3aa9e3)] px-6 text-[13px] font-extrabold tracking-[0.08em] text-[#052235] uppercase hover:brightness-[1.06] disabled:cursor-wait disabled:opacity-[0.65] max-[640px]:min-h-[42px] max-[640px]:px-[14px] max-[640px]:text-[11px]"
										type="button"
										onclick={openFilePreview}
									>
										<Eye class="size-4" aria-hidden="true" />
										Preview file
									</button>
								{/if}
								{#if permissions.download}
									<button
										class="inline-flex min-h-12 cursor-pointer items-center justify-center gap-3 rounded-[4px] border-0 bg-[linear-gradient(180deg,#4ab8f0,#3aa9e3)] px-6 text-[13px] font-extrabold tracking-[0.08em] text-[#052235] uppercase hover:brightness-[1.06] disabled:cursor-wait disabled:opacity-[0.65] max-[640px]:min-h-[42px] max-[640px]:px-[14px] max-[640px]:text-[11px]"
										type="button"
										disabled={startingDownload}
										onclick={() => downloadSharedFile()}
									>
										<Download class="size-4" aria-hidden="true" />
										{startingDownload ? 'Choose save location…' : 'Download file'}
									</button>
								{/if}
								{#if !permissions.preview && !permissions.download}
									<p class="m-0 leading-[1.55] text-[#87929a]">
										This link has no available actions.
									</p>
								{/if}
								{#if error}<p class="m-0 leading-[1.5] text-red-400" role="alert">{error}</p>{/if}
							</div>
						{/if}
					</div>
				</div>
			</section>
		{/if}

		<footer class="flex-[0_0_auto] px-3 pt-2 text-center text-xs text-[#6f7983]">
			<p class="m-0">
				Secure, self-hosted file sharing. Powered by <strong class="text-[#dee3e8]"
					>Arkive Core.</strong
				>
			</p>
		</footer>
	</div>
</main>

<FileEntryContextMenu
	mode="public"
	open={contextMenuOpen}
	entry={contextMenuEntry}
	x={contextMenuPosition.x}
	y={contextMenuPosition.y}
	allowPreview={permissions.preview}
	allowDownload={permissions.download}
	onClose={() => (contextMenuEntry = null)}
	onOpenFolder={openFolderById}
	onDownload={downloadSharedFile}
	onPreviewImage={(entry) => void openFolderFile(entry)}
	onPreviewMedia={(entry) => void openFolderFile(entry)}
	onPreviewPdf={(entry) => void openFolderFile(entry)}
	onPreviewText={(entry) => void openFolderFile(entry)}
	onProperties={() => {}}
	onDelete={() => {}}
	onCreateFolder={() => {}}
	onCreateTextFile={() => {}}
	onUploadFiles={() => {}}
	onUploadFolder={() => {}}
/>

{#if previewOpen && permissions.preview && shareEntry && fileReader}
	{#if isImageFile(shareEntry.name, shareEntry.type)}
		<ImageViewer
			entries={[shareEntry]}
			request={imagePreviewRequest}
			reader={fileReader}
			onClose={closePreview}
			onError={(message) => (error = message)}
		/>
	{:else if isPdfFile(shareEntry)}
		<PdfViewer entry={shareEntry} reader={fileReader} onClose={closePreview} />
	{:else if isAudioMimeType(shareEntry.type) || shareEntry.type.startsWith('video/')}
		<MediaViewer entry={shareEntry} reader={fileReader} onClose={closePreview} />
	{/if}
{/if}
