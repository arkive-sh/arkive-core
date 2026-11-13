<script lang="ts">
	import { onMount } from 'svelte';
	import { Folder } from '@lucide/svelte';

	type Props = {
		variant: 'list' | 'grid';
		name?: string;
		busy: boolean;
		placeholder?: string;
		showFolderIcon?: boolean;
		selectOnMount?: boolean;
		selectBeforeExtension?: boolean;
		onSubmit: () => void | Promise<void>;
		onCancel: () => void;
	};

	let {
		variant,
		name = $bindable(''),
		busy,
		placeholder = 'New folder name',
		showFolderIcon = true,
		selectOnMount = false,
		selectBeforeExtension = false,
		onSubmit,
		onCancel,
	}: Props = $props();
	let container = $state<HTMLFormElement>();
	let input = $state<HTMLInputElement>();

	onMount(() => {
		input?.focus();
		if (selectOnMount) {
			const extensionStart = name.lastIndexOf('.');
			const selectionEnd =
				selectBeforeExtension && extensionStart > 0 && extensionStart < name.length - 1
					? extensionStart
					: name.length;
			input?.setSelectionRange(0, selectionEnd);
		}
	});

	function handleOutsidePointer(event: PointerEvent): void {
		if (!busy && container && event.target instanceof Node && !container.contains(event.target))
			onCancel();
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (event.key === 'Escape') {
			event.preventDefault();
			event.stopPropagation();
			if (!busy) onCancel();
		} else if (event.key === 'Enter') {
			event.preventDefault();
			event.stopPropagation();
			if (!busy && name.trim()) void onSubmit();
		}
	}

	function submit(event: SubmitEvent): void {
		event.preventDefault();
		if (!busy && name.trim()) void onSubmit();
	}
</script>

<svelte:window onpointerdown={handleOutsidePointer} />

<form
	bind:this={container}
	class={`relative z-10 ${variant === 'list' ? 'flex min-w-0 flex-1 items-center gap-2' : 'grid min-w-0 gap-1.5'}`}
	aria-busy={busy}
	onsubmit={submit}
>
	{#if variant === 'list' && showFolderIcon}<Folder
			class="size-4 shrink-0 text-(--accent)"
			aria-hidden="true"
		/>{/if}
	<input
		bind:this={input}
		bind:value={name}
		class="h-8 min-w-0 flex-1 rounded border border-(--border) bg-(--bg) px-2 text-sm text-(--text) outline-none placeholder:text-(--muted) focus:border-(--accent) focus:ring-1 focus:ring-(--accent)"
		type="text"
		{placeholder}
		aria-label={placeholder}
		disabled={busy}
		onkeydown={handleKeydown}
	/>
</form>
