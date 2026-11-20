<script lang="ts">
	import * as v from 'valibot';
	import Button from '#lib/components/Button.svelte';
	import InputField from '#lib/components/InputField.svelte';
	import type { MFAChallenge, MFAProof } from '#lib/opaque-auth.ts';
	import { recoveryCodeSchema, totpCodeSchema } from '#lib/validation/totp.ts';

	type Props = {
		challenge: MFAChallenge;
		invalidCode: boolean;
		busy: boolean;
		error: string;
		onVerify: (proof: MFAProof) => void;
		onCancel: () => void;
	};
	let { challenge, invalidCode, busy, error, onVerify, onCancel }: Props = $props();
	let useRecoveryCode = $state(false);
	let rememberDevice = $state(false);
	let validationError = $state('');

	function submit(event: SubmitEvent): void {
		event.preventDefault();
		if (!(event.currentTarget instanceof HTMLFormElement)) return;
		const value = String(new FormData(event.currentTarget).get('mfa-proof') ?? '').trim();
		const result = v.safeParse(
			challenge.totpSetup || !useRecoveryCode ? totpCodeSchema : recoveryCodeSchema,
			value,
		);

		if (!result.success) {
			validationError = result.issues[0]?.message ?? 'Enter a valid verification code.';

			return;
		}
		validationError = '';
		onVerify({
			...(challenge.totpSetup || !useRecoveryCode ? { code: value } : { recoveryCode: value }),
			rememberDevice,
		});
	}
</script>

<div class="grid gap-3">
	{#if challenge.totpSetup}
		<div class="grid justify-items-center gap-3 text-center">
			<p class="m-0 text-sm leading-5 text-(--text)">
				Your password was reset, so set up your authenticator again to continue.
			</p>
			<img
				class="size-48 rounded border border-(--border) bg-white p-2"
				src={challenge.totpSetup.qrCodeDataURL}
				alt="Authenticator setup QR code"
			/>
			<p class="m-0 text-left text-xs leading-4.5 text-(--muted)">
				Can’t scan it? Enter this key manually:
				<code class="mt-1 block break-all font-mono text-sm text-(--text)"
					>{challenge.totpSetup.secret}</code
				>
			</p>
		</div>
	{/if}
	{#if error || invalidCode}
		<p class="m-0 text-sm text-(--danger)" role="alert">
			{error || 'That code was not accepted. Try again.'}
		</p>
	{/if}
	<form class="grid gap-3" onsubmit={submit}>
		<InputField
			label={challenge.totpSetup
				? 'Authenticator code'
				: useRecoveryCode
					? 'Recovery code'
					: 'Authenticator code'}
			name="mfa-proof"
			placeholder={useRecoveryCode ? 'Enter a recovery code' : '6-digit code'}
			inputmode={useRecoveryCode ? 'text' : 'numeric'}
			autocomplete="one-time-code"
			maxlength={useRecoveryCode ? 35 : 6}
			hasError={!!validationError}
			helperText={validationError}
			oninput={() => (validationError = '')}
			required
		/>
		<label class="flex items-start gap-2 text-sm leading-5 text-(--text)">
			<input
				class="mt-1 size-4 shrink-0 accent-(--accent)"
				type="checkbox"
				bind:checked={rememberDevice}
			/>
			<span
				>Remember this device for 30 days. You’ll still need your password to unlock the vault.</span
			>
		</label>
		<Button
			text="Verify and unlock"
			variant="primary"
			type="submit"
			{busy}
			busyText="Verifying…"
			class="w-full"
		/>
	</form>
	<div class="flex flex-wrap justify-between gap-3 text-sm">
		{#if !challenge.totpSetup}
			<button
				class="cursor-pointer border-0 bg-transparent p-0 text-(--accent-cool) underline underline-offset-2"
				type="button"
				onclick={() => (useRecoveryCode = !useRecoveryCode)}
			>
				{useRecoveryCode ? 'Use authenticator code' : 'Use a recovery code'}
			</button>
		{/if}
		<button
			class="cursor-pointer border-0 bg-transparent p-0 text-(--muted) underline underline-offset-2"
			type="button"
			disabled={busy}
			onclick={onCancel}>Cancel</button
		>
	</div>
</div>
