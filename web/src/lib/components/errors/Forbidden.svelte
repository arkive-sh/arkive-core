<script lang="ts">
	import { ArrowLeft, BadgeCheck, Lock, Shield } from '@lucide/svelte';
	import Button from '#lib/components/Button.svelte';
</script>

<div class="forbidden-page">
	<div class="forbidden-bg-texture"></div>
	<div class="forbidden-scanline"></div>
	<main class="forbidden-shell">
		<div class="forbidden-lock-wrap">
			<div class="forbidden-lock-glow"></div>
			<div class="forbidden-lock-card">
				<Lock class="forbidden-lock-icon" aria-hidden="true" />
				<div class="forbidden-corner forbidden-corner-top"></div>
				<div class="forbidden-corner forbidden-corner-bottom"></div>
			</div>
		</div>
		<section class="forbidden-copy">
			<p class="forbidden-eyebrow">Error Code: 403_FORBIDDEN</p>
			<h1 class="forbidden-title">403: Access Denied</h1>
			<div class="forbidden-rule"></div>
			<p class="forbidden-lead">You do not have permission to access this resource.</p>
			<div class="forbidden-actions">
				<Button text="Dashboard" variant="primary" href="/dashboard" class="forbidden-action-primary">
					{#snippet icon()}<Shield size="18" class="icon" aria-hidden="true" />{/snippet}
				</Button>
				<Button text="Login" variant="secondary" href="/login" class="forbidden-action-secondary">
					{#snippet icon()}
						<ArrowLeft class="button-lucide" aria-hidden="true" />
					{/snippet}
				</Button>
			</div>
			<div class="forbidden-footer">
				<p class="forbidden-footer-copy">Powered by <span class="forbidden-footer-strong">Arkive Core</span></p>
				<BadgeCheck class="forbidden-footer-icon" aria-hidden="true" />
			</div>
		</section>
	</main>
</div>
