<script lang="ts">
	import axios from 'axios';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import * as v from 'valibot';
	import { api, type APIErrorResponse } from '#lib/api.ts';
	import { opaqueAuth } from '#lib/opaque-auth.ts';
	import Button from '#lib/components/Button.svelte';
	import Card from '#lib/components/Card.svelte';
	import InputField from '#lib/components/InputField.svelte';
	import { callVaultWorker } from '#lib/vault/worker-client.ts';

	interface RecoveryVault {
		userID: string;
		encryptedMasterKeyRecovery: string;
	}

	const passwordSchema = v.pipe(
		v.string(),
		v.nonEmpty('password is required'),
		v.minBytes(8, 'password must be at least 8 characters'),
		v.regex(/\p{Ll}/u, 'password must include a lowercase letter'),
		v.regex(/\p{Lu}/u, 'password must include an uppercase letter'),
		v.regex(/[\p{P}\p{S}]/u, 'password must include a symbol')
	);

	let token = $state('');
	let vault = $state<RecoveryVault | null>(null);
	let error = $state('');
	let loading = $state(true);
	let submitting = $state(false);

	onMount(() => {
		token = new URLSearchParams(window.location.search).get('token') ?? '';
		if (!token) {
			error = 'Recovery token is missing.';
			loading = false;
			return;
		}
		void loadVault();
	});

	async function loadVault() {
		try {
			const { data } = await api.get<RecoveryVault>(`/api/auth/password-reset/vault?token=${encodeURIComponent(token)}`);
			vault = data;
		} catch (cause) {
			error = axios.isAxiosError<APIErrorResponse>(cause)
				? cause.response?.data?.error?.message ?? cause.message
				: 'Password recovery could not be loaded.';
		} finally {
			loading = false;
		}
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (!vault || submitting) return;
		const form = event.currentTarget;
		if (!(form instanceof HTMLFormElement)) return;
		let values: FormData | null = new FormData(form);
		let recoveryKey: string | null = String(values.get('recovery_key') ?? '').trim();
		let password: string | null = String(values.get('new_password') ?? '');
		let confirmPassword: string | null = String(values.get('confirm_password') ?? '');
		error = '';
		const passwordCheck = v.safeParse(passwordSchema, password);
		if (!passwordCheck.success) {
			error = passwordCheck.issues[0]?.message ?? 'Enter a valid password.';
			return;
		}
		if (password !== confirmPassword) {
			error = 'Passwords do not match.';
			return;
		}

		submitting = true;
		values = null;
		let registration: Awaited<ReturnType<typeof opaqueAuth.registerPasswordReset>> | null = null;
		let exportKey: Uint8Array<ArrayBuffer> | null = null;
		let recoveryKeyBytes: Uint8Array<ArrayBuffer> | null = null;
		try {
			if (password === null || recoveryKey === null) throw new Error('Recovery details are unavailable.');
			recoveryKeyBytes = new TextEncoder().encode(recoveryKey);
			recoveryKey = null;
			registration = await opaqueAuth.registerPasswordReset(token, password);
			password = null;
			confirmPassword = null;
			exportKey = registration.exportKey;
			const registrationRecord = registration.registrationRecord;
			registration = null;
			let prepared: { vaultSalt: string; encryptedMasterKey: string; recoveryProof: string };
			try {
				prepared = await callVaultWorker(
					'preparePasswordReset',
					{
						encryptedMasterKeyRecovery: vault.encryptedMasterKeyRecovery,
						recoveryKey: recoveryKeyBytes.buffer,
						exportKey: exportKey.buffer,
						userID: vault.userID,
						resetToken: token
					},
					[exportKey.buffer, recoveryKeyBytes.buffer]
				);
			} finally {
				if (exportKey?.byteLength) exportKey.fill(0);
				exportKey = null;
				if (recoveryKeyBytes?.byteLength) recoveryKeyBytes.fill(0);
				recoveryKeyBytes = null;
			}
			await api.post('/api/auth/password-reset/complete', {
				token,
				opaqueRegistrationRecord: registrationRecord,
				vaultSalt: prepared.vaultSalt,
				encryptedMasterKey: prepared.encryptedMasterKey,
				recoveryProof: prepared.recoveryProof
			});
			await goto('/login?password-reset-complete=1', { replaceState: true });
		} catch (cause) {
			error = axios.isAxiosError<APIErrorResponse>(cause)
				? cause.response?.data?.error?.details?.fields?.general ?? cause.response?.data?.error?.message ?? cause.message
				: cause instanceof Error ? cause.message : 'Password recovery failed.';
		} finally {
			if (exportKey?.byteLength) exportKey.fill(0);
			exportKey = null;
			if (recoveryKeyBytes?.byteLength) recoveryKeyBytes.fill(0);
			recoveryKeyBytes = null;
			recoveryKey = null;
			password = null;
			confirmPassword = null;
			values = null;
			registration = null;
			form.reset();
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Arkive · Set a new password</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<main class="flex min-h-screen items-center justify-center bg-(--bg) px-4 py-8 text-(--text)">
	<div class="grid w-[min(420px,92vw)] gap-6">
		<header class="text-center">
			<h1 class="m-0 text-2xl font-semibold">Set a new password</h1>
			<p class="mt-2 text-sm text-(--muted)">Your password and recovery key stay in this browser.</p>
		</header>
		<Card class="w-full! rounded-lg! border-(--border-soft)! bg-(--surface-raised)! p-7!">
			{#if loading}
				<p class="m-0 text-sm text-(--muted)">Loading recovery details...</p>
			{:else}
				<form class="grid gap-4" onsubmit={submit}>
					{#if error}<p class="m-0 rounded border border-(--danger) bg-(--danger)/10 px-3 py-2 text-sm text-(--danger)" role="alert">{error}</p>{/if}
					<InputField label="Recovery key" name="recovery_key" required />
					<InputField label="New password" name="new_password" type="password" required />
					<InputField label="Confirm password" name="confirm_password" type="password" required />
					<Button text="Reset password" type="submit" variant="primary" busy={submitting} busyText="Resetting..." class="w-full!" />
				</form>
			{/if}
		</Card>
	</div>
</main>
