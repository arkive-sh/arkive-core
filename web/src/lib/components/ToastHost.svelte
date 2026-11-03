<script lang="ts">
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { CircleAlert, CircleCheckBig, Info, TriangleAlert, X } from '@lucide/svelte';
	import { TOAST_EVENT, type ToastDetail, type ToastType } from '#lib/toast.ts';

	type ToastItem = ToastDetail & { id: number; timer?: number };

	let items = $state<ToastItem[]>([]);
	let nextId = 0;

	const iconColors: Record<ToastType, string> = {
		success: 'text-emerald-400',
		warning: 'text-amber-400',
		error: 'text-red-400',
		info: 'text-sky-400',
	};

	function dismiss(id: number): void {
		const item = items.find((entry) => entry.id === id);

		if (item?.timer) window.clearTimeout(item.timer);
		items = items.filter((entry) => entry.id !== id);
	}

	function receiveToast(event: Event): void {
		const detail = (event as CustomEvent<ToastDetail>).detail;

		if (!detail?.message) return;
		const item: ToastItem = { ...detail, id: ++nextId };

		if ((detail.duration ?? 4000) > 0) {
			item.timer = window.setTimeout(() => dismiss(item.id), detail.duration ?? 4000);
		}
		items = [...items, item];
	}

	onMount(() => {
		window.addEventListener(TOAST_EVENT, receiveToast);

		return () => {
			window.removeEventListener(TOAST_EVENT, receiveToast);
			for (const item of items) {
				if (item.timer) window.clearTimeout(item.timer);
			}
		};
	});
</script>

<div
	class="pointer-events-none fixed top-4 right-4 z-100 flex w-[min(21rem,calc(100vw-2rem))] flex-col gap-2 max-[640px]:top-3 max-[640px]:right-3"
	aria-live="polite"
	aria-atomic="false"
>
	{#each items as item (item.id)}
		<div
			class="pointer-events-auto flex items-start gap-2.5 rounded-lg border border-(--border) bg-(--surface-raised) px-3 py-2.5 text-(--text) shadow-(--shadow)"
			role={item.type === 'error' ? 'alert' : 'status'}
			transition:fly={{ y: -6, duration: 160 }}
		>
			{#if item.type === 'success'}
				<CircleCheckBig
					class={`mt-0.5 size-4 shrink-0 ${iconColors[item.type]}`}
					aria-hidden="true"
				/>
			{:else if item.type === 'warning'}
				<TriangleAlert
					class={`mt-0.5 size-4 shrink-0 ${iconColors[item.type]}`}
					aria-hidden="true"
				/>
			{:else if item.type === 'error'}
				<CircleAlert class={`mt-0.5 size-4 shrink-0 ${iconColors[item.type]}`} aria-hidden="true" />
			{:else}
				<Info class={`mt-0.5 size-4 shrink-0 ${iconColors[item.type]}`} aria-hidden="true" />
			{/if}
			<div class="min-w-0 flex-1">
				{#if item.title}<p class="m-0 mb-0.5 text-xs leading-4 font-semibold">{item.title}</p>{/if}
				<p class="m-0 wrap-break-word text-sm leading-5 text-(--muted)">{item.message}</p>
			</div>
			<button
				class="inline-flex size-6 shrink-0 cursor-pointer items-center justify-center rounded text-(--muted-strong) hover:bg-(--surface-highest) hover:text-(--text)"
				type="button"
				aria-label="Dismiss notification"
				onclick={() => dismiss(item.id)}
			>
				<X class="size-3.5" aria-hidden="true" />
			</button>
		</div>
	{/each}
</div>
