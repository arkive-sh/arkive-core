<script lang="ts">
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>Arkive · Dashboard</title>
</svelte:head>

<div class="min-h-[calc(100vh-57px)] px-6 py-8 md:px-8">
	<div class="mx-auto w-full max-w-7xl space-y-6">
		<header>
			<h1 class="text-2xl leading-8 font-semibold tracking-[-0.02em]">Welcome back, {data.user.brand_name || 'Arkive Core'}</h1>
			<p class="mt-2 text-(--muted)">Your private file server is ready.</p>
		</header>

		<section class="grid gap-3 sm:grid-cols-2" aria-label="Account overview">
			<article class="rounded-md border border-(--border) bg-(--surface-raised) p-5">
				<p class="text-[11px] leading-4 font-semibold tracking-wider text-(--muted) uppercase">Instance</p>
				<p class="mt-3 truncate text-xl font-semibold text-(--accent-strong)">
					{data.user.brand_name || 'Arkive Core'}
				</p>
			</article>
			<article class="rounded-md border border-(--border) bg-(--surface-raised) p-5">
				<p class="text-[11px] leading-4 font-semibold tracking-wider text-(--muted) uppercase">Signed in as</p>
				<p class="mt-3 truncate font-mono text-xl font-semibold text-(--accent-strong)">{data.user.email}</p>
			</article>
		</section>
	</div>
</div>
