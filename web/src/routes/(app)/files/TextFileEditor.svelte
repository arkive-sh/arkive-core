<script lang="ts">
	import { onDestroy, onMount, tick } from 'svelte';
	import { SvelteMap } from 'svelte/reactivity';
	import { get } from 'svelte/store';
	import { createVirtualizer } from '@tanstack/svelte-virtual';
	import { Pencil, X } from '@lucide/svelte';
	import Modal from '#lib/components/Modal.svelte';
	import FileTypeIcon from '#lib/components/FileTypeIcon.svelte';
	import {
		useTransferManager,
		type UploadItem,
		type UploadSource,
	} from '#lib/transfers/manager.ts';
	import { DecryptedFileReader } from '#lib/vault/read-file.ts';
	import { formatBytes } from '#lib/utils/format.ts';
	import TextEditorLine from './TextEditorLine.svelte';
	import type { FileManagerEntry } from './types';

	const READ_CHUNK_SIZE = 1024 * 1024;
	const MAX_TEXT_FILE_BYTES = 128 * 1024 * 1024;
	const MAX_TEXT_LINE_BYTES = 1024 * 1024;
	const MAX_TEXT_LINES = 500_000;
	// shortcut: these editor limits keep the line index bounded; use paged text editing if larger files become a requirement.
	const encoder = new TextEncoder();
	// Map Unicode Bidi_Control code points (UAX #9) to labels; this displays controls and does not implement the bidi algorithm.
	const bidiControlNames = new SvelteMap<number, string>([
		[0x061c, 'ALM'],
		[0x200e, 'LRM'],
		[0x200f, 'RLM'],
		[0x202a, 'LRE'],
		[0x202b, 'RLE'],
		[0x202c, 'PDF'],
		[0x202d, 'LRO'],
		[0x202e, 'RLO'],
		[0x2066, 'LRI'],
		[0x2067, 'RLI'],
		[0x2068, 'FSI'],
		[0x2069, 'PDI'],
	]);

	let {
		entry,
		vaultId,
		parentFolderId,
		fileReader,
		readOnly = false,
		onClose,
	}: {
		entry: FileManagerEntry | null;
		vaultId?: string;
		parentFolderId?: string | null;
		fileReader?: DecryptedFileReader;
		readOnly?: boolean;
		onClose: () => void;
	} = $props();

	const transfers = useTransferManager();
	let lines = $state<Uint8Array[]>([]);
	let contentByteCount = $state(0);
	let loadedPlaintextBytes = $state(0);
	let totalPlaintextSize = $state<number | null>(null);
	let bidiControlCount = $state(0);
	let showHiddenCharacters = $state(false);
	let wrapLines = $state(true);
	let allLinesSelected = $state(false);
	let selectionMessage = $state('');
	let filename = $state('Untitled.txt');
	let editing = $state(false);
	let dirty = $state(false);
	let loading = $state(true);
	let errorMessage = $state('');
	let viewport = $state<HTMLDivElement>();
	let pendingFocus = $state<{ index: number; offset: number } | null>(null);
	let reader: DecryptedFileReader | undefined;
	let ownsReader = false;
	let controller: AbortController | undefined;
	let handedOff = false;
	const textSize = $derived(contentByteCount + Math.max(0, lines.length - 1));
	const virtualizer = createVirtualizer<HTMLDivElement, HTMLDivElement>({
		count: 0,
		getScrollElement: () => viewport ?? null,
		estimateSize: () => 28,
		overscan: 16,
		getItemKey: (index) => index,
	});
	const virtualRows = $derived($virtualizer.getVirtualItems());
	const totalRowsHeight = $derived($virtualizer.getTotalSize());

	function measureTextRow(element: HTMLDivElement, index: number) {
		const measure = () => {
			element.dataset.index = String(index);
			get(virtualizer).measureElement(element);
		};
		measure();

		return {
			update(nextIndex: number) {
				index = nextIndex;
				measure();
			},
		};
	}

	function toggleWrapLines(): void {
		wrapLines = !wrapLines;
		allLinesSelected = false;
		selectionMessage = '';
		get(virtualizer).measure();
	}

	function toggleHiddenCharacters(): void {
		showHiddenCharacters = !showHiddenCharacters;
		allLinesSelected = false;
		selectionMessage = '';
	}

	function selectAllLines(event: KeyboardEvent): boolean {
		if (!(event.ctrlKey || event.metaKey) || event.key.toLowerCase() !== 'a') return false;
		event.preventDefault();
		if (loading) {
			allLinesSelected = false;
			selectionMessage = 'Wait for the file to finish loading before selecting all text.';
		} else {
			allLinesSelected = true;
			selectionMessage = '';
		}

		return true;
	}

	function replaceSelectedLines(event: KeyboardEvent): boolean {
		if (!allLinesSelected || !editing) return false;
		if ((event.ctrlKey || event.metaKey) && ['c', 'x', 'v'].includes(event.key.toLowerCase()))
			return true;

		let replacement: string[];
		let focusIndex = 0;
		let focusOffset = 0;

		if (event.key === 'Backspace' || event.key === 'Delete') {
			replacement = [''];
		} else if (event.key === 'Enter') {
			replacement = ['', ''];
			focusIndex = 1;
		} else if (
			!event.ctrlKey &&
			!event.metaKey &&
			!event.altKey &&
			[...event.key].length === 1 &&
			!['Dead', 'Process', 'Unidentified'].includes(event.key)
		) {
			replacement = [event.key];
			focusOffset = event.key.length;
		} else {
			allLinesSelected = false;

			return false;
		}

		event.preventDefault();
		if (replaceLines(0, lines.length, replacement)) focusLine(focusIndex, focusOffset);
		allLinesSelected = false;

		return true;
	}

	function handleDocumentKeydown(event: KeyboardEvent): void {
		if (selectAllLines(event)) return;
		if ((event.ctrlKey || event.metaKey) && ['c', 'x', 'v'].includes(event.key.toLowerCase()))
			return;
		if (showHiddenCharacters && replaceSelectedLines(event)) return;
		if (event.key === 'Escape' || allLinesSelected) allLinesSelected = false;
	}

	function handleCopy(event: ClipboardEvent): void {
		if (!allLinesSelected) return;
		event.preventDefault();
		const decoder = new TextDecoder();
		let text = '';

		for (let index = 0; index < lines.length; index++) {
			const line = decoder.decode(lines[index]);
			text += showHiddenCharacters ? visibleBidiMarkers(line) : line;
			if (index < lines.length - 1) text += '\n';
		}
		event.clipboardData?.setData('text/plain', text);
	}

	function handleCut(event: ClipboardEvent): void {
		if (!allLinesSelected) return;
		handleCopy(event);
		if (!editing) return;
		if (replaceLines(0, lines.length, [''])) focusLine(0, 0);
		allLinesSelected = false;
	}

	function pastedLines(event: ClipboardEvent): string[] {
		return (event.clipboardData?.getData('text/plain') ?? '').replace(/\r\n?/g, '\n').split('\n');
	}

	function handleDocumentPaste(event: ClipboardEvent): void {
		if (!allLinesSelected || !editing || !showHiddenCharacters) return;
		event.preventDefault();
		const parts = pastedLines(event);

		if (replaceLines(0, lines.length, parts)) focusLine(parts.length - 1, parts.at(-1)!.length);
		allLinesSelected = false;
	}

	function handleDocumentPointerdown(): void {
		allLinesSelected = false;
		selectionMessage = '';
		if (!editing || showHiddenCharacters) viewport?.focus({ preventScroll: true });
	}

	function copyName(name: string): string {
		const extensionAt = name.lastIndexOf('.');

		return extensionAt > 0
			? `${name.slice(0, extensionAt)} (copy)${name.slice(extensionAt)}`
			: `${name} (copy).txt`;
	}

	function decodedLine(index: number): string {
		return new TextDecoder().decode(lines[index]);
	}

	function countBidiControls(value: string): number {
		let count = 0;

		for (const character of value) {
			if (bidiControlNames.has(character.codePointAt(0)!)) count++;
		}

		return count;
	}

	function visibleBidiMarkers(value: string): string {
		return [...value]
			.map((character) => {
				const codePoint = character.codePointAt(0)!;
				const name = bidiControlNames.get(codePoint);

				return name
					? `⟦U+${codePoint.toString(16).toUpperCase().padStart(4, '0')} ${name}⟧`
					: character;
			})
			.join('');
	}

	function addLoadedLine(value: string): void {
		const bytes = encoder.encode(value);

		if (bytes.byteLength > MAX_TEXT_LINE_BYTES || lines.length >= MAX_TEXT_LINES) {
			bytes.fill(0);

			throw new Error('This text file has a line that is too large to edit.');
		}
		contentByteCount += bytes.byteLength;
		bidiControlCount += countBidiControls(value);
		if (loading && bidiControlCount > 0) showHiddenCharacters = true;
		lines.push(bytes);
	}

	async function loadFile(): Promise<void> {
		if (!entry) return;
		controller = new AbortController();
		ownsReader = !fileReader;
		reader = fileReader ?? new DecryptedFileReader(entry.id, controller.signal);
		let pending = '';
		const decoder = new TextDecoder('utf-8', { fatal: true });
		const decode = (bytes?: Uint8Array, stream = false) => {
			try {
				return bytes ? decoder.decode(bytes, { stream }) : decoder.decode();
			} catch {
				throw new Error('This file is not valid UTF-8 text.');
			}
		};
		const appendDecoded = (value: string) => {
			pending += value;
			let newlineAt = pending.indexOf('\n');

			while (newlineAt >= 0) {
				const line = pending.slice(0, newlineAt).replace(/\r$/, '');
				addLoadedLine(line);
				pending = pending.slice(newlineAt + 1);
				newlineAt = pending.indexOf('\n');
			}
			const pendingBytes = encoder.encode(pending);
			const tooLong = pendingBytes.byteLength > MAX_TEXT_LINE_BYTES;
			pendingBytes.fill(0);
			if (tooLong) throw new Error('This text file has a line that is too large to edit.');
		};

		try {
			const { plaintextSize } = await reader.load();

			if (plaintextSize > MAX_TEXT_FILE_BYTES)
				throw new Error('Text files larger than 128 MiB cannot be opened in the editor.');
			totalPlaintextSize = plaintextSize;
			for (let start = 0; start < plaintextSize; start += READ_CHUNK_SIZE) {
				const bytes = await reader.readRange(
					start,
					Math.min(plaintextSize, start + READ_CHUNK_SIZE),
					controller.signal,
				);

				try {
					appendDecoded(decode(bytes, true));
					loadedPlaintextBytes = Math.min(plaintextSize, start + bytes.byteLength);
				} finally {
					bytes.fill(0);
				}
			}
			appendDecoded(decode());
			addLoadedLine(pending.replace(/\r$/, ''));
			showHiddenCharacters = bidiControlCount > 0;
		} catch (error) {
			clearLines();
			errorMessage =
				error instanceof TypeError
					? 'This file is not valid UTF-8 text.'
					: error instanceof Error
						? error.message
						: 'Could not open this text file.';
		} finally {
			if (ownsReader) {
				await reader?.dispose();
				reader = undefined;
			}
			loading = false;
			selectionMessage = '';
		}
	}

	function clearLines(): void {
		for (const line of lines) line.fill(0);
		lines = [];
		contentByteCount = 0;
		bidiControlCount = 0;
		loadedPlaintextBytes = 0;
		totalPlaintextSize = null;
	}

	function replaceLines(index: number, deleteCount: number, values: string[]): boolean {
		if (values.length === 0 || lines.length - deleteCount + values.length > MAX_TEXT_LINES) {
			errorMessage = 'This edit would exceed the text editor limit.';

			return false;
		}
		const replacement = values.map((value) => encoder.encode(value));

		if (replacement.some((line) => line.byteLength > MAX_TEXT_LINE_BYTES)) {
			for (const line of replacement) line.fill(0);
			errorMessage = 'A line cannot be larger than 1 MiB.';

			return false;
		}
		const removed = lines.slice(index, index + deleteCount);
		const removedBidiControls = removed.reduce(
			(sum, line) => sum + countBidiControls(new TextDecoder().decode(line)),
			0,
		);
		const insertedBidiControls = values.reduce((sum, value) => sum + countBidiControls(value), 0);
		const nextContentBytes =
			contentByteCount -
			removed.reduce((sum, line) => sum + line.byteLength, 0) +
			replacement.reduce((sum, line) => sum + line.byteLength, 0);
		const nextLineCount = lines.length - deleteCount + replacement.length;

		if (nextContentBytes + nextLineCount - 1 > MAX_TEXT_FILE_BYTES) {
			for (const line of replacement) line.fill(0);
			errorMessage = 'This edit would exceed the 128 MiB editor limit.';

			return false;
		}
		for (const line of removed) line.fill(0);
		lines.splice(index, deleteCount, ...replacement);
		if (deleteCount !== replacement.length) get(virtualizer).measure();
		contentByteCount = nextContentBytes;
		bidiControlCount += insertedBidiControls - removedBidiControls;
		dirty = true;
		errorMessage = '';

		return true;
	}

	function focusLine(index: number, offset: number): void {
		if (index < 0 || index >= lines.length) return;
		pendingFocus = { index, offset };
	}

	function updateLine(index: number, value: string): boolean {
		return replaceLines(index, 1, [value]);
	}

	function handleLineKeydown(index: number, event: KeyboardEvent): void {
		if (
			!editing ||
			!(
				event.currentTarget instanceof HTMLInputElement ||
				event.currentTarget instanceof HTMLTextAreaElement
			)
		)
			return;
		const input = event.currentTarget;
		const value = input.value;
		const start = input.selectionStart ?? value.length;
		const end = input.selectionEnd ?? start;

		if (selectAllLines(event)) return;
		if (replaceSelectedLines(event)) return;

		if (event.key === 'Enter') {
			event.preventDefault();
			if (replaceLines(index, 1, [value.slice(0, start), value.slice(end)]))
				focusLine(index + 1, 0);
		} else if (event.key === 'Backspace' && start === 0 && end === 0 && index > 0) {
			event.preventDefault();
			const previous = decodedLine(index - 1);

			if (replaceLines(index - 1, 2, [previous + value])) focusLine(index - 1, previous.length);
		} else if (
			event.key === 'Delete' &&
			start === value.length &&
			end === start &&
			index < lines.length - 1
		) {
			event.preventDefault();
			if (replaceLines(index, 2, [value + decodedLine(index + 1)])) focusLine(index, start);
		} else if (!wrapLines && event.key === 'ArrowUp' && start === end && index > 0) {
			event.preventDefault();
			focusLine(index - 1, start);
		} else if (
			!wrapLines &&
			event.key === 'ArrowDown' &&
			start === end &&
			index < lines.length - 1
		) {
			event.preventDefault();
			focusLine(index + 1, start);
		} else if (!wrapLines && event.key === 'ArrowLeft' && start === 0 && end === 0 && index > 0) {
			event.preventDefault();
			focusLine(index - 1, decodedLine(index - 1).length);
		} else if (
			!wrapLines &&
			event.key === 'ArrowRight' &&
			start === value.length &&
			end === start &&
			index < lines.length - 1
		) {
			event.preventDefault();
			focusLine(index + 1, 0);
		}
	}

	function handleLinePaste(index: number, event: ClipboardEvent): void {
		if (
			!editing ||
			!(
				event.currentTarget instanceof HTMLInputElement ||
				event.currentTarget instanceof HTMLTextAreaElement
			)
		)
			return;
		event.preventDefault();
		const input = event.currentTarget;
		const value = input.value;
		const start = input.selectionStart ?? value.length;
		const end = input.selectionEnd ?? start;
		const parts = pastedLines(event);

		if (allLinesSelected) {
			if (replaceLines(0, lines.length, parts)) focusLine(parts.length - 1, parts.at(-1)!.length);
			allLinesSelected = false;

			return;
		}
		parts[0] = value.slice(0, start) + parts[0];
		parts[parts.length - 1] += value.slice(end);
		if (replaceLines(index, 1, parts))
			focusLine(index + parts.length - 1, parts.at(-1)!.length - value.slice(end).length);
	}

	function makeUploadSource(name: string, sourceLines: Uint8Array[]): UploadSource {
		const offsets = new Float64Array(sourceLines.length + 1);

		for (let index = 0; index < sourceLines.length; index++) {
			offsets[index + 1] =
				offsets[index]! + sourceLines[index]!.byteLength + (index < sourceLines.length - 1 ? 1 : 0);
		}
		const size = offsets.at(-1) ?? 0;
		let disposed = false;

		return {
			name,
			type: 'text/plain;charset=utf-8',
			size,
			lastModified: Date.now(),
			slice(start, end) {
				return {
					async arrayBuffer() {
						if (disposed) throw new Error('Text upload data has been cleared.');
						const from = Math.max(0, Math.min(size, start));
						const to = Math.max(from, Math.min(size, end));
						const bytes = new Uint8Array(to - from);
						let low = 0;
						let high = sourceLines.length;

						while (low < high) {
							const middle = (low + high) >>> 1;

							if (offsets[middle + 1]! <= from) low = middle + 1;
							else high = middle;
						}
						for (let index = low; index < sourceLines.length && offsets[index]! < to; index++) {
							const lineStart = offsets[index]!;
							const line = sourceLines[index]!;
							const copyStart = Math.max(from, lineStart);
							const copyEnd = Math.min(to, lineStart + line.byteLength);

							if (copyEnd > copyStart)
								bytes.set(
									line.subarray(copyStart - lineStart, copyEnd - lineStart),
									copyStart - from,
								);
							const newlineAt = lineStart + line.byteLength;

							if (index < sourceLines.length - 1 && newlineAt >= from && newlineAt < to)
								bytes[newlineAt - from] = 10;
						}

						return bytes.buffer;
					},
				};
			},
			dispose() {
				if (disposed) return;
				disposed = true;
				for (const line of sourceLines) line.fill(0);
				sourceLines.length = 0;
				offsets.fill(0);
			},
		};
	}

	function saveCopy(): void {
		const name = filename.trim();

		if (!vaultId) {
			errorMessage = 'A vault is required to save a copy.';

			return;
		}

		if (!name || /[/\\\p{Cc}]/u.test(name) || name === '.' || name === '..') {
			errorMessage = 'Enter a valid file name without path separators.';

			return;
		}
		const source = makeUploadSource(name, [...lines]);
		const item: UploadItem = { file: source, relativePath: name };

		if (!transfers.enqueueUploads([item], vaultId, parentFolderId ?? null)) {
			errorMessage = 'Could not add the text file to transfers.';

			return;
		}
		lines = [];
		contentByteCount = 0;
		handedOff = true;
		onClose();
	}

	function requestClose(): void {
		if (dirty && !window.confirm('Discard your unsaved text changes?')) return;
		onClose();
	}

	$effect(() => {
		const count = lines.length;
		const scrollElement = viewport;
		const current = get(virtualizer);

		if (current.options.count !== count || current.options.getScrollElement() !== scrollElement) {
			current.setOptions({ count, getScrollElement: () => scrollElement ?? null });
		}
	});

	$effect(() => {
		const target = pendingFocus;

		if (!target || !viewport) return;
		void tick()
			.then(() => {
				if (pendingFocus !== target) return;
				get(virtualizer).scrollToIndex(target.index, { align: 'auto' });

				return tick();
			})
			.then(() => {
				if (pendingFocus !== target) return;
				const input = viewport?.querySelector<HTMLTextAreaElement>(
					`[data-line-input="${target.index}"]`,
				);
				input?.focus();
				input?.setSelectionRange(
					Math.min(target.offset, input.value.length),
					Math.min(target.offset, input.value.length),
				);
				pendingFocus = null;
			});
	});

	onMount(() => {
		if (entry) {
			filename = copyName(entry.name);
			editing = false;
			void loadFile();
		} else {
			lines = [new Uint8Array()];
			editing = true;
			loading = false;
			void tick().then(() => focusLine(0, 0));
		}
	});

	onDestroy(() => {
		controller?.abort();
		if (ownsReader) void reader?.dispose();
		if (!handedOff) clearLines();
		filename = '';
		errorMessage = '';
	});
</script>

<Modal
	open
	label={entry ? `Text file ${entry.name}` : 'New text file'}
	onClose={requestClose}
	dialogClass="!h-[88vh] !max-h-[90vh] !w-[min(1100px,100%)] !overflow-hidden"
	bodyClass="!flex !min-h-0 !flex-1 !grid-rows-[minmax(0,1fr)] !gap-0 !p-0"
>
	{#snippet header()}
		<header class="flex min-w-0 items-center gap-3 border-b border-(--border) px-5 py-3">
			<FileTypeIcon
				name={entry?.name ?? filename}
				mime={entry?.type ?? 'text/plain'}
				className="size-5 shrink-0"
			/>
			<div class="min-w-0 flex-1">
				<h2 class="m-0 truncate text-base font-semibold">{entry ? entry.name : 'New text file'}</h2>
				<p class="m-0 text-xs text-(--muted)">
					{loading
						? totalPlaintextSize === null
							? 'Loading…'
							: `${formatBytes(loadedPlaintextBytes)} of ${formatBytes(totalPlaintextSize)} loaded`
						: `${formatBytes(textSize)} · ${lines.length.toLocaleString()} lines`}
				</p>
			</div>
			{#if entry && !readOnly && !editing && !errorMessage && !loading}
				<button
					class={`h-9 cursor-pointer rounded border px-3 text-sm hover:bg-(--surface-highest) ${wrapLines ? 'border-(--accent)' : 'border-(--border)'}`}
					type="button"
					aria-pressed={wrapLines}
					onclick={toggleWrapLines}>Wrap lines</button
				>
				{#if bidiControlCount > 0}
					<button
						class="h-9 cursor-pointer rounded border border-(--border) px-3 text-sm hover:bg-(--surface-highest)"
						type="button"
						onclick={toggleHiddenCharacters}
						>{showHiddenCharacters ? 'Hide character markers' : 'Show hidden characters'}</button
					>
				{/if}
				<button
					class="inline-flex size-9 cursor-pointer items-center justify-center rounded border border-(--border) hover:bg-(--surface-highest)"
					type="button"
					aria-label="Edit a copy"
					onclick={() => {
						editing = true;
						filename = copyName(entry.name);
						showHiddenCharacters = false;
						allLinesSelected = false;
						selectionMessage = '';
					}}><Pencil class="size-4" aria-hidden="true" /></button
				>
			{/if}
			<button
				class="inline-flex size-9 cursor-pointer items-center justify-center rounded border border-(--border) hover:bg-(--surface-highest)"
				type="button"
				aria-label="Close text editor"
				onclick={requestClose}><X class="size-4" aria-hidden="true" /></button
			>
		</header>
	{/snippet}
	<div class="flex min-h-0 flex-1 flex-col">
		{#if editing}
			<div class="flex flex-wrap items-center gap-2 border-b border-(--border) px-4 py-3">
				<input
					class="h-9 min-w-0 flex-1 rounded border border-(--border) bg-(--bg) px-3 text-sm text-(--text) outline-none focus:border-(--accent)"
					aria-label="Text file name"
					bind:value={filename}
					oninput={() => (dirty = true)}
				/>
				<button
					class={`h-9 cursor-pointer rounded border px-3 text-sm hover:bg-(--surface-highest) ${wrapLines ? 'border-(--accent)' : 'border-(--border)'}`}
					type="button"
					aria-pressed={wrapLines}
					onclick={toggleWrapLines}>Wrap lines</button
				>
				{#if bidiControlCount > 0}<button
						class="h-9 cursor-pointer rounded border border-(--border) px-3 text-sm hover:bg-(--surface-highest)"
						type="button"
						aria-pressed={showHiddenCharacters}
						onclick={toggleHiddenCharacters}
						>{showHiddenCharacters ? 'Hide character markers' : 'Show hidden characters'}</button
					>{/if}
				<button
					class="h-9 cursor-pointer rounded border border-(--accent) bg-(--accent) px-3 text-sm font-semibold text-(--accent-ink) disabled:cursor-not-allowed disabled:opacity-50"
					type="button"
					disabled={loading || (!dirty && entry !== null)}
					onclick={saveCopy}>{entry ? 'Save copy' : 'Save file'}</button
				>
			</div>
		{/if}
		{#if bidiControlCount > 0}
			<p class="m-0 border-b border-(--border) px-4 py-2 text-xs text-amber-300" role="status">
				{bidiControlCount} hidden formatting character{bidiControlCount === 1 ? '' : 's'} can make nearby
				text appear in a different order.
				{showHiddenCharacters
					? ' Each is marked inline, for example ⟦U+202E RLO⟧.'
					: ' Select “Show hidden characters” to inspect them.'}
			</p>
		{/if}
		{#if allLinesSelected}<p
				class="m-0 border-b border-(--border) px-4 py-2 text-xs text-(--muted)"
				role="status"
			>
				{editing
					? 'All text selected. Copy, replace, or delete it.'
					: 'All text selected. Copy to place it on the clipboard.'}
			</p>{/if}
		{#if selectionMessage}<p
				class="m-0 border-b border-(--border) px-4 py-2 text-xs text-(--muted)"
				role="status"
			>
				{selectionMessage}
			</p>{/if}
		{#if errorMessage}<p
				class="m-0 border-b border-(--border) px-4 py-2 text-sm text-red-400"
				role="alert"
			>
				{errorMessage}
			</p>{/if}
		{#if lines.length}
			{#if loading}<p
					class="m-0 border-b border-(--border) px-4 py-2 text-xs text-(--muted)"
					role="status"
				>
					Loading and decrypting text. You can read the lines loaded so far.
				</p>{/if}
			<!-- eslint-disable-next-line svelte/no-unused-svelte-ignore -->
			<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions: the virtual text region needs focus and keyboard handlers for document-level selection. -->
			<div
				bind:this={viewport}
				class="min-h-0 flex-1 overflow-auto bg-(--bg) py-2 focus-visible:outline focus-visible:outline-2 focus-visible:outline-(--accent)"
				tabindex={editing && !showHiddenCharacters ? -1 : 0}
				role="region"
				aria-label="Text document"
				onkeydown={handleDocumentKeydown}
				oncopy={handleCopy}
				oncut={handleCut}
				onpaste={handleDocumentPaste}
				onpointerdown={handleDocumentPointerdown}
			>
				<div class="relative w-full" style={`height:${totalRowsHeight}px`}>
					{#each virtualRows as row (row.key)}
						<div
							use:measureTextRow={row.index}
							class={`absolute left-0 flex min-h-7 items-start ${wrapLines ? 'w-full' : 'w-max min-w-full'} ${allLinesSelected ? 'bg-(--surface-highest)' : ''}`}
							style={`transform:translateY(${row.start}px)`}
						>
							<span
								class="w-14 shrink-0 select-none pr-3 text-right font-mono text-xs leading-7 text-(--muted)"
								aria-hidden="true">{row.index + 1}</span
							>
							{#if editing && !showHiddenCharacters}
								<TextEditorLine
									index={row.index}
									bytes={lines[row.index]!}
									{wrapLines}
									onChange={updateLine}
									onKeydown={handleLineKeydown}
									onPaste={handleLinePaste}
								/>
							{:else}
								<pre
									class={`m-0 px-2 font-mono text-sm leading-7 text-(--text) ${wrapLines ? 'min-w-0 flex-1 whitespace-pre-wrap wrap-break-word' : 'w-max whitespace-pre'}`}>{showHiddenCharacters
										? visibleBidiMarkers(decodedLine(row.index))
										: decodedLine(row.index) || ' '}</pre>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{:else if loading}
			<div class="grid flex-1 place-items-center text-sm text-(--muted)" role="status">
				Decrypting text file…
			</div>
		{:else if !errorMessage}
			<div class="grid flex-1 place-items-center text-sm text-(--muted)">
				This text file is empty.
			</div>
		{/if}
	</div>
</Modal>
