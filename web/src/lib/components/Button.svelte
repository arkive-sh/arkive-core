<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	type Props = Omit<HTMLButtonAttributes, 'class' | 'type' | 'disabled' | 'onclick'> & {
		text: string;
		variant?: string;
		type?: HTMLButtonAttributes['type'];
		href?: string;
		class?: string;
		icon?: Snippet;
		busyText?: string;
		busy?: boolean;
		disabled?: boolean;
		onclick?: HTMLButtonAttributes['onclick'];
	};

	let {
		text,
		variant = '',
		type = 'button',
		href,
		class: className = '',
		id,
		icon,
		busyText,
		busy = false,
		disabled = false,
		onclick,
		...buttonProps
	}: Props = $props();

	const variants: Record<string, string> = {
		primary:
			'border-(--accent) bg-(--accent) text-(--accent-ink) hover:border-(--accent-strong) hover:bg-(--accent-strong)',
		secondary:
			'border-(--border) bg-transparent text-(--text) shadow-none hover:bg-(--surface-highest)',
		danger: 'border-(--border) bg-[rgba(147,0,10,0.18)] text-(--danger) shadow-none',
		'danger-outline':
			'border-[rgba(147,0,10,0.55)] bg-transparent text-(--danger) shadow-none hover:border-[rgba(147,0,10,0.7)] hover:bg-[rgba(147,0,10,0.16)]',
	};
	let classes = $derived(
		[
			'inline-flex min-h-10 cursor-pointer items-center justify-center gap-2 rounded-md border px-4 text-sm font-semibold leading-5 [transition:background-color_0.2s_ease,border-color_0.2s_ease,color_0.2s_ease,box-shadow_0.2s_ease] disabled:cursor-not-allowed disabled:opacity-[0.72]',
			variants[variant],
			className,
			busy && 'cursor-progress!',
		]
			.filter(Boolean)
			.join(' '),
	);
</script>

{#if href}
	<a {...buttonProps} class={classes} {href} {id}>
		{#if icon}<span class="inline-flex items-center justify-center">{@render icon()}</span>{/if}
		<span>{text}</span>
	</a>
{:else}
	<button
		{...buttonProps}
		class={classes}
		{type}
		{id}
		data-busy-text={busyText}
		disabled={disabled || busy}
		aria-busy={busy}
		{onclick}
	>
		{#if busy}
			<span
				class="size-4 shrink-0 animate-[spin_0.7s_linear_infinite] rounded-full border-2 border-current border-r-transparent"
				aria-hidden="true"
			></span>
		{:else if icon}
			<span class="inline-flex items-center justify-center">{@render icon()}</span>
		{/if}
		<span>{busy && busyText ? busyText : text}</span>
	</button>
{/if}
