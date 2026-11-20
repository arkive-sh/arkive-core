<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { useQueryClient } from '@tanstack/svelte-query';
	import { Key, Lock, Shield, ShieldCheck } from '@lucide/svelte';
	import { loginAndUnlock } from '#lib/auth.ts';
	import Button from '#lib/components/Button.svelte';
	import Card from '#lib/components/Card.svelte';
	import InputField from '#lib/components/InputField.svelte';
	import MFAChallengeForm from '#lib/components/MFAChallengeForm.svelte';
	import type { MFAChallenge, MFAProof } from '#lib/opaque-auth.ts';

	const queryClient = useQueryClient();
	type LoginPhase = 'credentials' | 'mfa' | 'recovery-codes';
	let phase = $state<LoginPhase>('credentials');
	let generalError = $state('');
	let submitting = $state(false);
	let checkingMFA = $state(false);
	let invalidMFA = $state(false);
	let challenge = $state<MFAChallenge | null>(null);
	let recoveryCodes = $state<string[]>([]);
	let mfaResolve: ((proof: MFAProof) => void) | null = null;
	let mfaReject: ((error: Error) => void) | null = null;
	let cancelled = false;

	function requestMFA(nextChallenge: MFAChallenge, invalidCode: boolean): Promise<MFAProof> {
		challenge = nextChallenge;
		invalidMFA = invalidCode;
		checkingMFA = false;
		phase = 'mfa';

		return new Promise((resolve, reject) => {
			mfaResolve = resolve;
			mfaReject = reject;
		});
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (submitting) return;

		const form = event.currentTarget as HTMLFormElement;
		const passwordInput = form.elements.namedItem('password') as HTMLInputElement;
		const email = String(new FormData(form).get('email') || '').trim();
		let password: string | null = passwordInput.value;
		passwordInput.value = '';
		generalError = '';
		submitting = true;

		try {
			queryClient.clear();
			if (password === null) throw new Error('Password is unavailable.');
			const loginPromise = loginAndUnlock(email, password, requestMFA);
			password = null;
			const codes = await loginPromise;

			if (codes.length > 0) {
				recoveryCodes = codes;
				phase = 'recovery-codes';
			} else {
				await goto(resolve('/(app)'), { replace: true });
			}
		} catch (cause) {
			if (!cancelled) {
				generalError = cause instanceof Error ? cause.message : 'Unlock failed. Try again.';
				phase = 'credentials';
			}
		} finally {
			password = null;
			passwordInput.value = '';
			submitting = false;
			checkingMFA = false;
			challenge = null;
			mfaResolve = null;
			mfaReject = null;
			cancelled = false;
		}
	}

	function submitMFA(proof: MFAProof): void {
		if (!mfaResolve) return;
		checkingMFA = true;
		generalError = '';
		mfaResolve(proof);
		mfaResolve = null;
	}

	function cancelMFA(): void {
		cancelled = true;
		mfaReject?.(new Error('Login cancelled.'));
		mfaResolve = null;
		mfaReject = null;
		challenge = null;
		phase = 'credentials';
		checkingMFA = false;
	}

	async function copyRecoveryCodes(): Promise<void> {
		try {
			await navigator.clipboard.writeText(recoveryCodes.join('\n'));
			generalError = '';
		} catch {
			generalError = 'Could not copy the recovery codes.';
		}
	}

	async function finishRecoveryCodes(): Promise<void> {
		recoveryCodes = [];
		await goto(resolve('/(app)'), { replace: true });
	}
</script>

<svelte:head>
	<title>Arkive · Admin Login</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<div class="flex h-screen flex-col overflow-hidden">
	<div class="flex min-h-0 flex-1 items-center justify-center bg-(--bg) px-4.5 py-10">
		<div class="grid w-[min(420px,92vw)] gap-7">
			<header class="grid justify-items-center gap-2.5 text-center">
				<span class="inline-flex items-center justify-center text-(--accent-cool)">
					{#if phase === 'credentials'}
						<Lock class="size-8 stroke-2" aria-hidden="true" />
					{:else}
						<ShieldCheck class="size-8 stroke-2" aria-hidden="true" />
					{/if}
				</span>
				<h1 class="m-0 text-2xl leading-8 font-semibold">
					{phase === 'credentials'
						? 'Arkive Core'
						: phase === 'mfa'
							? 'Verify it’s you'
							: 'Save recovery codes'}
				</h1>
				<p class="m-0 text-[13px] leading-4.5 text-(--muted)">
					{phase === 'credentials'
						? 'Sign in to your self-hosted encrypted file server.'
						: phase === 'mfa'
							? 'Enter the code from your authenticator app to finish signing in.'
							: 'These recovery codes are shown only once. Save them somewhere safe.'}
				</p>
			</header>

			<Card
				class="w-full! rounded-lg! border-[color-mix(in_srgb,var(--border)_88%,white_8%)]! bg-[color-mix(in_srgb,var(--surface)_88%,black)]! p-7.5! shadow-[0_10px_30px_rgba(0,0,0,0.32)]!"
			>
				{#if phase === 'credentials'}
					<form class="grid gap-4" method="POST" onsubmit={submit}>
						{#if generalError}
							<p
								class="m-0 rounded border border-[rgba(147,0,10,0.3)] bg-[rgba(147,0,10,0.18)] px-3.5 py-3 text-[13px] leading-4.5 text-(--danger)"
								role="alert"
							>
								{generalError}
							</p>
						{/if}

						<InputField
							label="Email"
							name="email"
							type="email"
							placeholder="user@local.node"
							required
							fieldClass="mb-0! gap-2.5!"
							labelClass="text-[color-mix(in_srgb,var(--muted)_92%,white_12%)]!"
							inputClass="rounded! border-[color-mix(in_srgb,var(--border)_92%,white_8%)]! bg-[color-mix(in_srgb,var(--bg)_92%,black)]! px-3.5! py-3! font-mono text-[13px]! leading-4.5! shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]! focus:border-(--accent-cool)! focus:shadow-[0_0_0_1px_rgba(107,209,230,0.55)]!"
						/>
						<InputField
							label="Password"
							name="password"
							type="password"
							placeholder="Enter your password"
							required
							fieldClass="mb-0! gap-2.5!"
							labelClass="text-[color-mix(in_srgb,var(--muted)_92%,white_12%)]!"
							inputClass="rounded! border-[color-mix(in_srgb,var(--border)_92%,white_8%)]! bg-[color-mix(in_srgb,var(--bg)_92%,black)]! px-3.5! py-3! font-mono text-[13px]! leading-4.5! shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]! focus:border-(--accent-cool)! focus:shadow-[0_0_0_1px_rgba(107,209,230,0.55)]!"
						>
							{#snippet suffix()}
								<Shield class="size-3.5 stroke-2" aria-hidden="true" />
							{/snippet}
						</InputField>

						<Button
							text="Login"
							variant="primary"
							type="submit"
							class="mt-2! min-h-13! w-full! rounded! border-0! bg-[#38bdf8]! text-[15px]! font-semibold! leading-5! text-[#0d1c2d]! shadow-none! hover:bg-[#63caf9]!"
							busyText="Logging in..."
							busy={submitting}
						>
							{#snippet icon()}
								<Key class="size-5 stroke-2" aria-hidden="true" />
							{/snippet}
						</Button>
						<p
							class="m-0 rounded bg-[rgba(107,209,230,0.12)] px-3.5 py-3 text-[13px] leading-4.5 text-(--accent-cool)"
						>
							<a
								href={resolve('/forgot-password')}
								class="underline decoration-current underline-offset-2">Forgot password?</a
							>
						</p>
					</form>
				{:else if phase === 'mfa' && challenge}
					<MFAChallengeForm
						{challenge}
						invalidCode={invalidMFA}
						busy={checkingMFA}
						error={generalError}
						onVerify={submitMFA}
						onCancel={cancelMFA}
					/>
				{:else}
					<div class="grid gap-3">
						{#if generalError}
							<p class="m-0 text-sm text-(--danger)" role="alert">{generalError}</p>
						{/if}
						<p class="m-0 text-sm leading-5 text-(--text)">
							Two-factor authentication is set up. Each code works once if you lose access to your
							authenticator.
						</p>
						<ul
							class="m-0 grid list-none grid-cols-1 gap-1 rounded-md border border-(--border) p-4 font-mono text-sm text-(--text) sm:grid-cols-2"
						>
							{#each recoveryCodes as code (code)}<li>{code}</li>{/each}
						</ul>
						<Button text="Copy recovery codes" variant="secondary" onclick={copyRecoveryCodes} />
						<Button
							text="I’ve saved them — continue"
							variant="primary"
							onclick={finishRecoveryCodes}
						/>
					</div>
				{/if}
			</Card>

			<footer
				class="inline-flex items-center justify-center gap-2.5 font-mono text-[13px] leading-4.5 text-[color-mix(in_srgb,var(--muted)_82%,white_4%)]"
			>
				<span
					class="size-1.5 rounded-full bg-(--accent-cool) shadow-[0_0_0_3px_rgba(107,209,230,0.08)]"
				></span>
				<span>System standing by.</span>
			</footer>
		</div>
	</div>
</div>
