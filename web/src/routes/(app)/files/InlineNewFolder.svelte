<script lang="ts">
	import { onMount } from 'svelte';
	import { Folder } from '@lucide/svelte';

	type Props = {
		variant: 'list' | 'grid';
		name?: string;
		busy: boolean;
		onCreate: () => void | Promise<void>;
		onCancel: () => void;
	};

	let { variant, name = $bindable(''), busy, onCreate, onCancel }: Props = $props();
	let container = $state<HTMLFormElement>();
	let input = $state<HTMLInputElement>();

	onMount(() => input?.focus());

	function handleOutsidePointer(event: PointerEvent): void {
		if (!busy && container && event.target instanceof Node && !container.contains(event.target)) onCancel();
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (event.key === 'Escape') {
			event.preventDefault();
			event.stopPropagation();
			if (!busy) onCancel();
		} else if (event.key === 'Enter' && !busy && name.trim()) {
			event.preventDefault();
			void onCreate();
		}
	}

	function submit(event: SubmitEvent): void {
		event.preventDefault();
		if (!busy && name.trim()) void onCreate();
	}
</script>

<svelte:window onpointerdown={handleOutsidePointer} />

<form
	bind:this={container}
	class={variant === 'list' ? 'flex min-w-0 flex-1 items-center gap-2' : 'grid min-w-0 gap-1.5'}
	aria-busy={busy}
	onsubmit={submit}
>
	{#if variant === 'list'}<Folder class="size-4 shrink-0 text-(--accent)" aria-hidden="true" />{/if}
	<input
		bind:this={input}
		bind:value={name}
		class="h-8 min-w-0 flex-1 rounded border border-(--border) bg-(--bg) px-2 text-sm text-(--text) outline-none placeholder:text-(--muted) focus:border-(--accent) focus:ring-1 focus:ring-(--accent)"
		type="text"
		placeholder="New folder name"
		aria-label="New folder name"
		disabled={busy}
		onkeydown={handleKeydown}
	/>
</form>
