<script lang="ts">
	import { BadgeCheck, Search, Unlink } from '@lucide/svelte';
	import Button from '#lib/components/Button.svelte';

	let { status }: { status: number } = $props();
	let notFound = $derived(status === 404);
</script>

<main class="not-found">
	<div class="not-found-grid-overlay"></div>
	<div class="not-found-shell">
		<div class="not-found-visual">
			<div class="not-found-visual-card">
				<Unlink class="not-found-visual-icon" aria-hidden="true" />
				<div class="not-found-corner not-found-corner-top-left"></div>
				<div class="not-found-corner not-found-corner-top-right"></div>
				<div class="not-found-corner not-found-corner-bottom-left"></div>
				<div class="not-found-corner not-found-corner-bottom-right"></div>
			</div>
			<div class="not-found-code-row">
				<span class="not-found-code-dot"></span>
				<span class="not-found-code-text">{notFound ? 'ERROR_CODE: PATH_UNRESOLVED' : `ERROR_CODE: HTTP_${status}`}</span>
			</div>
		</div>
		<section class="not-found-copy">
			<h1 class="not-found-title">{notFound ? '404: Node Path Not Found' : `${status}: Request Failed`}</h1>
			<p class="not-found-lead">
				{notFound
					? 'The requested resource does not exist on this node or has been moved. The address may be invalid or no longer available.'
					: 'The request could not be completed. Please try again or return to the dashboard.'}
			</p>
			<div class="not-found-actions">
				<Button text="Return to Dashboard" variant="primary" href="/dashboard" class="not-found-action-primary" />
				<Button text="Open Files" variant="secondary" href="/files" class="not-found-action-secondary">
					{#snippet icon()}<Search size="18" class="icon" aria-hidden="true" />{/snippet}
				</Button>
			</div>
			<div class="not-found-footer">
				<p class="not-found-footer-copy">Powered by <span class="not-found-footer-strong">Arkive Core</span></p>
				<BadgeCheck class="not-found-footer-icon" aria-hidden="true" />
			</div>
		</section>
	</div>
</main>
