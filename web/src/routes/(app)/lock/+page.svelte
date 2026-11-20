<script lang="ts">
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { onMount } from 'svelte';
	import { LockKeyhole } from '@lucide/svelte';
	import Button from '#lib/components/Button.svelte';
	import InputField from '#lib/components/InputField.svelte';
	import MFAChallengeForm from '#lib/components/MFAChallengeForm.svelte';
	import LogoutButton from '#lib/components/LogoutButton.svelte';
	import { loginAndUnlock } from '#lib/auth.ts';
	import { vaultUnlocked } from '#lib/vault/state.ts';
	import type { MFAChallenge, MFAProof } from '#lib/opaque-auth.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let password = $state('');
	let error = $state('');
	let submitting = $state(false);
	let checkingMFA = $state(false);
	let challenge = $state<MFAChallenge | null>(null);
	let invalidMFA = $state(false);
	let recoveryCodes = $state<string[]>([]);
	let cancelled = false;
	let mfaResolve: ((proof: MFAProof) => void) | null = null;
	let mfaReject: ((error: Error) => void) | null = null;

	onMount(() => {
		if ($vaultUnlocked) void goto(resolve('/(app)'), { replace: true });
	});

	function requestMFA(nextChallenge: MFAChallenge, invalidCode: boolean): Promise<MFAProof> {
		challenge = nextChallenge;
		invalidMFA = invalidCode;
		checkingMFA = false;

		return new Promise((resolve, reject) => {
			mfaResolve = resolve;
			mfaReject = reject;
		});
	}

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (submitting || !password) return;
		submitting = true;
		error = '';
		let submittedPassword = password;
		password = '';
		cancelled = false;
		try {
			const loginPromise = loginAndUnlock(data.user.email, submittedPassword, requestMFA);
			submittedPassword = '';
			recoveryCodes = await loginPromise;
			if (recoveryCodes.length === 0) await goto(resolve('/(app)'), { replace: true });
		} catch (cause) {
			if (!cancelled)
				error = cause instanceof Error ? cause.message : 'Could not unlock the vault.';
		} finally {
			submittedPassword = '';
			submitting = false;
			checkingMFA = false;
			challenge = null;
			mfaResolve = null;
			mfaReject = null;
		}
	}

	function verifyMFA(proof: MFAProof): void {
		if (!mfaResolve) return;
		checkingMFA = true;
		mfaResolve(proof);
		mfaResolve = null;
	}

	function cancelMFA(): void {
		cancelled = true;
		mfaReject?.(new Error('Login cancelled.'));
		mfaResolve = null;
		mfaReject = null;
		challenge = null;
		checkingMFA = false;
	}

	async function copyRecoveryCodes(): Promise<void> {
		try {
			await navigator.clipboard.writeText(recoveryCodes.join('\n'));
			error = '';
		} catch {
			error = 'Could not copy the recovery codes.';
		}
	}

	async function finishRecoveryCodes(): Promise<void> {
		recoveryCodes = [];
		await goto(resolve('/(app)'), { replace: true });
	}
</script>

<svelte:head>
	<title>Unlock vault | Arkive</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<section class="relative grid min-h-screen place-items-center px-4 py-10">
	<div class="absolute top-5 right-5">
		<LogoutButton side="left" />
	</div>
	<div class="w-full max-w-md">
		<div class="mb-5 flex size-12 items-center justify-center text-(--accent-cool)">
			<LockKeyhole class="size-7" aria-hidden="true" />
		</div>
		{#if recoveryCodes.length > 0}
			<h1 class="m-0 text-2xl leading-8 font-semibold text-(--text)">Save recovery codes</h1>
			<p class="mt-2 mb-6 text-sm leading-5 text-(--muted)">
				These codes are shown only once. Save them somewhere safe before continuing.
			</p>
			<ul
				class="m-0 grid list-none grid-cols-1 gap-1 rounded-md border border-(--border) p-4 font-mono text-sm text-(--text) sm:grid-cols-2"
			>
				{#each recoveryCodes as code (code)}<li>{code}</li>{/each}
			</ul>
			{#if error}<p class="mt-3 mb-0 text-sm text-(--danger)" role="alert">{error}</p>{/if}
			<div class="mt-4 flex gap-2">
				<Button text="Copy recovery codes" variant="secondary" onclick={copyRecoveryCodes} />
				<Button text="I’ve saved them — continue" variant="primary" onclick={finishRecoveryCodes} />
			</div>
		{:else if challenge}
			<h1 class="m-0 text-2xl leading-8 font-semibold text-(--text)">Verify it’s you</h1>
			<p class="mt-2 mb-6 text-sm leading-5 text-(--muted)">
				Enter an authenticator or recovery code to unlock this device.
			</p>
			<MFAChallengeForm
				{challenge}
				invalidCode={invalidMFA}
				busy={checkingMFA}
				{error}
				onVerify={verifyMFA}
				onCancel={cancelMFA}
			/>
		{:else}
			<h1 class="m-0 text-2xl leading-8 font-semibold text-(--text)">Unlock your vault</h1>
			<p class="mt-2 mb-6 text-sm leading-5 text-(--muted)">
				Enter your password to unlock your files on this device.
			</p>
			<form class="grid gap-2" onsubmit={submit}>
				<InputField
					label="Password"
					name="password"
					type="password"
					autocomplete="current-password"
					required
					value={password}
					oninput={(event) => (password = event.currentTarget.value)}
				/>
				{#if error}
					<p class="m-0 text-sm text-(--danger)" role="alert">{error}</p>
				{/if}
				<Button
					text="Unlock vault"
					busyText="Unlocking…"
					type="submit"
					busy={submitting}
					disabled={!password}
					class="mt-2 w-full"
				/>
			</form>
		{/if}
	</div>
</section>
