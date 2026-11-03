<script lang="ts">
	import { page } from '$app/state';
	import Forbidden from '#lib/components/errors/Forbidden.svelte';
	import RequestError from '#lib/components/errors/RequestError.svelte';

	let title = $derived(
		page.status === 403
			? 'Arkive · Forbidden'
			: page.status === 404
				? `Arkive · ${page.url.pathname} not found`
				: 'Arkive · Error',
	);
</script>

<svelte:head>
	<title>{title}</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

{#if page.status === 403}
	<Forbidden />
{:else}
	<RequestError status={page.status} />
{/if}
