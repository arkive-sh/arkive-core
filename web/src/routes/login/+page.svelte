<script lang="ts">
	import { Key, Lock, Shield } from '@lucide/svelte';
	import { loginAndUnlock } from '#lib/auth.ts';
	import Button from '#lib/components/Button.svelte';
	import Card from '#lib/components/Card.svelte';
	import InputField from '#lib/components/InputField.svelte';

	let generalError = $state('');
	let submitting = $state(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (submitting) return;

		const form = event.currentTarget as HTMLFormElement;
		const passwordInput = form.elements.namedItem('password') as HTMLInputElement;
		const email = String(new FormData(form).get('email') || '').trim();
		const password = passwordInput.value;
		submitting = true;
		generalError = '';

		try {
			await loginAndUnlock(email, password);
			passwordInput.value = '';
			window.location.assign('/dashboard');
		} catch (cause) {
			generalError = cause instanceof Error ? cause.message : 'Unlock failed. Try again.';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Arkive · Admin Login</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="auth-page">
	<div class="auth-container">
		<div class="auth-shell">
			<header class="auth-header">
				<span class="auth-mark">
					<Lock class="auth-lucide auth-lucide-lock" aria-hidden="true" />
				</span>
				<h1>Arkive Core</h1>
				<p class="auth-subtitle">Sign in to your self-hosted encrypted file server.</p>
			</header>

			<Card class="auth-card auth-card-login">
				<form class="auth-form" method="POST" onsubmit={submit}>
					{#if generalError}<p class="form-error" role="alert">{generalError}</p>{/if}

					<InputField label="Email" name="email" type="email" placeholder="user@local.node" required inputClass="form-input-auth mono" />
					<InputField label="Password" name="password" type="password" placeholder="Enter your password" required inputClass="form-input-auth mono">
						{#snippet suffix()}
							<Shield class="auth-lucide auth-lucide-shield" aria-hidden="true" />
						{/snippet}
					</InputField>

					<Button text="Login" variant="primary" type="submit" class="auth-submit auth-submit-login" busyText="Logging in..." busy={submitting}>
						{#snippet icon()}
							<Key class="button-lucide" aria-hidden="true" />
						{/snippet}
					</Button>
					<p class="auth-message"><a href="/forgot-password">Forgot password?</a></p>
				</form>
			</Card>

			<footer class="auth-status">
				<span class="auth-status-dot"></span>
				<span>System standing by.</span>
			</footer>
		</div>
	</div>
</div>
