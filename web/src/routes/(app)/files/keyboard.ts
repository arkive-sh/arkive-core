import { fileManagerEntryKey, NEW_FOLDER_DRAFT_ID, type FileManagerEntry } from './types';

type DeleteShortcut = 'trash' | 'permanent';

type FilesPageKeyboardOptions = {
	blocked: boolean;
	rows: FileManagerEntry[];
	selectedEntries: FileManagerEntry[];
	clipboardCount: number;
	hasParent: boolean;
	onSelectAll: () => void;
	onDelete: (entry?: FileManagerEntry) => void;
	onPermanentDelete: (entry?: FileManagerEntry) => void;
	onNavigateBack: () => void;
	onCut: () => void;
	onPaste: () => void;
	onNewFolder: () => void;
};

type TrashPageKeyboardOptions = {
	blocked: boolean;
	rows: FileManagerEntry[];
	selectedIds: ReadonlySet<string>;
	selectedEntries: FileManagerEntry[];
	hasParent: boolean;
	onSelectAll: () => void;
	onNavigateBack: () => void;
	onPermanentDelete: (targets: FileManagerEntry[]) => void;
	onPermanentDeleteContents: (entry: FileManagerEntry) => void;
};

type FileManagerViewKeyboardOptions = {
	layout: 'list' | 'grid';
	rows: FileManagerEntry[];
	selectedIds: ReadonlySet<string>;
	columnCount?: number;
	focusEntry: (index: number) => void;
	onSelectEntry?: (entry: FileManagerEntry, additive: boolean) => void;
	getEntryActionLabel: (entry: FileManagerEntry) => string | undefined;
	openEntry: (entry: FileManagerEntry, trigger: HTMLElement) => void;
};

function getDeleteShortcut(event: KeyboardEvent): DeleteShortcut | null {
	if (
		event.key !== 'Delete' ||
		event.ctrlKey ||
		event.metaKey ||
		event.altKey ||
		!(event.target instanceof HTMLElement)
	)
		return null;

	return event.shiftKey ? 'permanent' : 'trash';
}

function getFocusedEntry(
	target: EventTarget | null,
	rows: FileManagerEntry[],
): FileManagerEntry | undefined {
	if (!(target instanceof Element)) return undefined;
	const rawIndex = target.closest<HTMLElement>('[data-entry-index]')?.dataset.entryIndex;

	if (rawIndex === undefined) return undefined;
	const index = Number(rawIndex);

	if (!Number.isInteger(index) || index < 0) return undefined;
	const entry = rows[index];

	return entry?.id === NEW_FOLDER_DRAFT_ID ? undefined : entry;
}

function isEditableTarget(target: HTMLElement, includeRadio = false): boolean {
	return (
		target.isContentEditable ||
		Boolean(
			target.closest(
				`input:not([type="checkbox"]${includeRadio ? ':not([type="radio"])' : ''}), textarea, select, [contenteditable="true"]`,
			),
		)
	);
}

function isViewBlockedTarget(target: HTMLElement): boolean {
	return Boolean(
		document.querySelector('[data-modal-open="true"]') ||
		target.closest('[aria-modal="true"], [role="menu"]'),
	);
}

export function createFilesPageKeyboardHandler(
	getOptions: () => FilesPageKeyboardOptions,
): (event: KeyboardEvent) => void {
	return (event) => {
		const target = event.target;

		if (event.defaultPrevented || event.repeat || !(target instanceof HTMLElement)) return;
		if (isEditableTarget(target, true)) return;
		const options = getOptions();

		if (options.blocked) return;

		const deleteShortcut = getDeleteShortcut(event);

		if (deleteShortcut) {
			event.preventDefault();
			const entry = getFocusedEntry(target, options.rows);

			if (deleteShortcut === 'trash') options.onDelete(entry);
			else options.onPermanentDelete(entry);

			return;
		}

		const command = event.ctrlKey || event.metaKey;

		if (command && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 'a') {
			event.preventDefault();
			options.onSelectAll();

			return;
		}
		if (
			(event.key === 'Backspace' || (event.altKey && event.key === 'ArrowLeft')) &&
			options.hasParent
		) {
			event.preventDefault();
			options.onNavigateBack();

			return;
		}

		if (command && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 'x') {
			if (options.selectedEntries.length) {
				event.preventDefault();
				options.onCut();
			}

			return;
		}
		if (command && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 'v') {
			if (options.clipboardCount) {
				event.preventDefault();
				options.onPaste();
			}

			return;
		}
		if (command && event.altKey && !event.shiftKey && event.key.toLowerCase() === 'n') {
			event.preventDefault();
			options.onNewFolder();
		}
	};
}

export function createTrashPageKeyboardHandler(
	getOptions: () => TrashPageKeyboardOptions,
): (event: KeyboardEvent) => void {
	return (event) => {
		const target = event.target;

		if (event.defaultPrevented || event.repeat || !(target instanceof HTMLElement)) return;
		if (isEditableTarget(target)) return;
		const options = getOptions();

		if (options.blocked) return;

		if (getDeleteShortcut(event) === 'permanent') {
			event.preventDefault();
			const focused = getFocusedEntry(target, options.rows);

			if (focused?.isTrashContainer) options.onPermanentDeleteContents(focused);
			else {
				const targets = focused
					? options.selectedIds.has(fileManagerEntryKey(focused)) && options.selectedEntries.length
						? options.selectedEntries
						: [focused]
					: options.selectedEntries;
				options.onPermanentDelete(targets);
			}

			return;
		}

		const command = event.ctrlKey || event.metaKey;

		if (command && !event.altKey && !event.shiftKey && event.key.toLowerCase() === 'a') {
			event.preventDefault();
			options.onSelectAll();

			return;
		}
		if (
			(event.key === 'Backspace' || (event.altKey && event.key === 'ArrowLeft')) &&
			options.hasParent
		) {
			event.preventDefault();
			options.onNavigateBack();
		}
	};
}

export function createFileManagerViewKeyboardHandler(
	getOptions: () => FileManagerViewKeyboardOptions,
): (event: KeyboardEvent) => void {
	return (event) => {
		const target = event.target;

		if (event.defaultPrevented || !(target instanceof HTMLElement)) return;
		if (isViewBlockedTarget(target) || isEditableTarget(target)) return;

		const options = getOptions();
		const { rows, selectedIds, focusEntry } = options;
		const row = target.closest<HTMLElement>('[data-entry-index]');

		if (!row) {
			const isArrow =
				options.layout === 'list'
					? event.key === 'ArrowDown' || event.key === 'ArrowUp'
					: ['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight'].includes(event.key);

			if (!isArrow || event.altKey) return;
			const selected = rows.findIndex(
				(entry) => entry.id !== NEW_FOLDER_DRAFT_ID && selectedIds.has(fileManagerEntryKey(entry)),
			);
			const first =
				selected >= 0 ? selected : rows.findIndex((entry) => entry.id !== NEW_FOLDER_DRAFT_ID);

			if (first >= 0) {
				event.preventDefault();
				focusEntry(first);
			}

			return;
		}

		const index = Number(row.dataset.entryIndex);
		const entry = rows[index];

		if (!entry) return;

		if (options.layout === 'list' && (event.key === 'ArrowDown' || event.key === 'ArrowUp')) {
			if (event.altKey) return;
			event.preventDefault();
			const navigableIndexes = rows.flatMap((entry, rowIndex) =>
				entry.id === NEW_FOLDER_DRAFT_ID ? [] : [rowIndex],
			);
			const direction = event.key === 'ArrowDown' ? 1 : -1;
			const current = navigableIndexes.indexOf(index);
			const next =
				navigableIndexes[
					current < 0 ? (direction > 0 ? 0 : navigableIndexes.length - 1) : current + direction
				];

			if (next !== undefined) focusEntry(next);
			else if (direction < 0) window.scrollTo({ top: 0, behavior: 'smooth' });
		} else if (
			options.layout === 'grid' &&
			['ArrowDown', 'ArrowUp', 'ArrowLeft', 'ArrowRight'].includes(event.key)
		) {
			if (event.altKey) return;
			event.preventDefault();
			const columnCount = Math.max(1, options.columnCount ?? 1);
			const step =
				event.key === 'ArrowDown'
					? columnCount
					: event.key === 'ArrowUp'
						? -columnCount
						: event.key === 'ArrowRight'
							? 1
							: -1;
			let next = index + step;

			while (rows[next]?.id === NEW_FOLDER_DRAFT_ID) next += Math.sign(step);
			if (event.key === 'ArrowDown' && next >= rows.length) next = rows.length - 1;
			if (next < 0 || next >= rows.length || next === index) {
				if (event.key === 'ArrowUp') window.scrollTo({ top: 0, behavior: 'smooth' });

				return;
			}
			focusEntry(next);
		} else if (
			(event.key === 'Enter' || event.code === 'Space') &&
			target.closest('[data-entry-action]')
		) {
			event.preventDefault();
			if (event.code === 'Space') {
				options.onSelectEntry?.(entry, event.ctrlKey || event.metaKey);

				return;
			}
			const action = target.closest<HTMLElement>('[data-entry-action]')!;

			if (options.getEntryActionLabel(entry)) options.openEntry(entry, action);
			else options.onSelectEntry?.(entry, event.ctrlKey || event.metaKey);
		}
	};
}

export function createContextMenuKeyboardHandler(
	getOptions: () => { open: boolean; menu?: HTMLDivElement; onClose: () => void },
): (event: KeyboardEvent) => void {
	return (event) => {
		const { open, menu, onClose } = getOptions();

		if (!open) return;
		if (event.key === 'Escape') {
			event.preventDefault();
			onClose();

			return;
		}
		if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) return;
		const items = Array.from(menu?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? []);

		if (!items.length) return;
		event.preventDefault();
		const currentIndex = items.indexOf(document.activeElement as HTMLButtonElement);
		const nextIndex =
			event.key === 'Home'
				? 0
				: event.key === 'End'
					? items.length - 1
					: (currentIndex + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
		items[nextIndex]?.focus();
	};
}
