<script lang="ts">
	import { onDestroy } from 'svelte';
	import type { Snippet } from 'svelte';

	let {
		label,
		children,
		disabled = false,
		className = '',
		side = 'right',
	}: {
		label: string;
		children: Snippet;
		disabled?: boolean;
		className?: string;
		side?: 'left' | 'right';
	} = $props();

	let root = $state<HTMLSpanElement>();
	let tooltip = $state<HTMLSpanElement>();
	let visible = $state(false);
	let pointerOver = false;
	let focusWithin = false;
	let position = $state({ left: 0, top: 0 });
	let listening = false;

	function positionTooltip(): void {
		if (!root || !tooltip) return;

		const trigger = root.getBoundingClientRect();
		const tip = tooltip.getBoundingClientRect();
		const margin = 8;
		const gap = 10;
		let actualSide = side;

		if (actualSide === 'right' && trigger.right + gap + tip.width > window.innerWidth - margin)
			actualSide = 'left';
		else if (actualSide === 'left' && trigger.left - gap - tip.width < margin) actualSide = 'right';

		const left = actualSide === 'right' ? trigger.right + gap : trigger.left - gap - tip.width;
		const top = trigger.top + (trigger.height - tip.height) / 2;
		position = {
			left: Math.round(Math.min(Math.max(left, margin), window.innerWidth - tip.width - margin)),
			top: Math.round(Math.min(Math.max(top, margin), window.innerHeight - tip.height - margin)),
		};
	}

	function updateVisibility(): void {
		const shouldShow = !disabled && (pointerOver || focusWithin);
		visible = shouldShow;
		if (shouldShow) {
			positionTooltip();
			if (!listening) {
				window.addEventListener('resize', positionTooltip);
				window.addEventListener('scroll', positionTooltip, true);
				listening = true;
			}
		} else if (listening) {
			window.removeEventListener('resize', positionTooltip);
			window.removeEventListener('scroll', positionTooltip, true);
			listening = false;
		}
	}

	function onFocusOut(event: FocusEvent): void {
		if (!(event.relatedTarget instanceof Node) || !root?.contains(event.relatedTarget)) {
			focusWithin = false;
			updateVisibility();
		}
	}

	onDestroy(() => {
		if (!listening) return;
		window.removeEventListener('resize', positionTooltip);
		window.removeEventListener('scroll', positionTooltip, true);
	});
</script>

<span
	bind:this={root}
	class={`relative inline-flex ${className}`}
	role="presentation"
	onpointerenter={() => {
		pointerOver = true;
		updateVisibility();
	}}
	onpointerleave={() => {
		pointerOver = false;
		updateVisibility();
	}}
	onfocusin={() => {
		focusWithin = true;
		updateVisibility();
	}}
	onfocusout={onFocusOut}
>
	{@render children()}
	{#if !disabled}
		<span
			bind:this={tooltip}
			class={`pointer-events-none fixed z-50 w-max max-w-[calc(100vw-1rem)] wrap-break-word rounded-md border border-(--border) bg-[color-mix(in_srgb,var(--surface-highest)_88%,black)] px-2.5 py-1.5 text-center text-xs font-medium text-(--text) shadow-lg transition-[opacity,visibility] duration-200 ease-out ${visible ? 'visible opacity-100' : 'invisible opacity-0'}`}
			style={`left: ${position.left}px; top: ${position.top}px`}
			role="tooltip"
		>
			{label}
		</span>
	{/if}
</span>
