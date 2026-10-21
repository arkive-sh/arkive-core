<script lang="ts">
	import type { Snippet } from 'svelte';

	type Props = {
		text: string;
		variant?: string;
		type?: 'button' | 'submit' | 'reset';
		href?: string;
		class?: string;
		id?: string;
		icon?: Snippet;
		busyText?: string;
		busy?: boolean;
		disabled?: boolean;
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
		disabled = false
	}: Props = $props();

	let classes = $derived(['button', variant, className, busy && 'is-busy'].filter(Boolean).join(' '));
</script>

{#if href}
	<a class={classes} {href} {id}>
		{#if icon}<span class="button-icon">{@render icon()}</span>{/if}
		<span class="button-label">{text}</span>
	</a>
{:else}
	<button
		class={classes}
		{type}
		{id}
		data-busy-text={busyText}
		disabled={disabled || busy}
		aria-busy={busy}
	>
		{#if busy && busyText}<span class="button-spinner" aria-hidden="true"></span>{/if}
		{#if icon}<span class="button-icon">{@render icon()}</span>{/if}
		<span class="button-label">{busy && busyText ? busyText : text}</span>
	</button>
{/if}
