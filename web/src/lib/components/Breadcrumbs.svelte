<script lang="ts">
	import { ChevronRight } from '@lucide/svelte';

	type BreadcrumbItem = {
		id: string;
		label: string;
		href?: string;
	};

	let { items, label = 'Breadcrumb' }: { items: BreadcrumbItem[]; label?: string } = $props();
	let breadcrumbList = $state<HTMLOListElement>();

	$effect(() => {
		const itemCount = items.length;
		if (breadcrumbList && itemCount) breadcrumbList.scrollLeft = breadcrumbList.scrollWidth;
	});
</script>

{#if items.length}
	<nav class="min-w-0" aria-label={label}>
		<ol bind:this={breadcrumbList} class="m-0 flex min-w-0 items-center gap-1 overflow-x-auto p-0 text-sm whitespace-nowrap">
			{#each items as item, index (item.id)}
				{#if index > 0}
					<li class="flex shrink-0 items-center text-(--muted)" aria-hidden="true">
						<ChevronRight class="size-3.5" />
					</li>
				{/if}
				<li class="min-w-0 shrink-0">
					{#if item.href}
						<a class="block max-w-48 truncate text-(--muted) transition-colors hover:text-(--text)" href={item.href}>{item.label}</a>
					{:else}
						<span class="block max-w-48 truncate font-medium text-(--text)" aria-current="page">{item.label}</span>
					{/if}
				</li>
			{/each}
		</ol>
	</nav>
{/if}
