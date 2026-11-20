<script lang="ts">
	import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query';
	import * as v from 'valibot';
	import { Copy, ShieldCheck } from '@lucide/svelte';
	import { api } from '#lib/api.ts';
	import Button from '#lib/components/Button.svelte';
	import InputField from '#lib/components/InputField.svelte';
	import { totpCodeSchema, totpProofSchema } from '#lib/validation/totp.ts';

	type TOTPSetup = { secret: string; qrCodeDataURL: string };
	type TOTPStatus = { enabled: boolean; recoveryCodesRemaining: number };
	type RecoveryCodes = { recoveryCodes: string[] };
	type DisableProof = { code: string } | { recoveryCode: string };

	const queryClient = useQueryClient();
	const statusQuery = createQuery(() => ({
		queryKey: ['settings', 'totp'],
		queryFn: async () => (await api.get<TOTPStatus>('/api/settings/totp')).data,
	}));
	let setup = $state<TOTPSetup | null>(null);
	let recoveryCodes = $state<string[]>([]);
	let localError = $state('');
	let validationError = $state('');
	let notice = $state('');

	const setupMutation = createMutation(() => ({
		mutationFn: async () => (await api.post<TOTPSetup>('/api/settings/totp/enroll')).data,
		onSuccess: (result) => {
			setup = result;
			notice = '';
		},
	}));
	const confirmMutation = createMutation(() => ({
		mutationFn: async (code: string) =>
			(await api.post<RecoveryCodes>('/api/settings/totp/confirm', { code })).data,
		onSuccess: async (result) => {
			setup = null;
			recoveryCodes = result.recoveryCodes;
			localError = '';
			validationError = '';
			notice =
				'Two-factor authentication is on. Save these recovery codes now; they will only be shown once.';
			queryClient.setQueryData<TOTPStatus>(['settings', 'totp'], {
				enabled: true,
				recoveryCodesRemaining: result.recoveryCodes.length,
			});
			await queryClient.invalidateQueries({ queryKey: ['settings', 'totp'] });
		},
	}));
	const disableMutation = createMutation(() => ({
		mutationFn: async (proof: DisableProof) => api.post('/api/settings/totp/disable', proof),
		onSuccess: async () => {
			recoveryCodes = [];
			localError = '';
			validationError = '';
			notice = 'Two-factor authentication is off.';
			queryClient.setQueryData<TOTPStatus>(['settings', 'totp'], {
				enabled: false,
				recoveryCodesRemaining: 0,
			});
			await queryClient.invalidateQueries({ queryKey: ['settings', 'totp'] });
		},
	}));

	let mutationError = $derived(
		setupMutation.error?.message ??
			confirmMutation.error?.message ??
			disableMutation.error?.message ??
			'',
	);
	let error = $derived(localError || mutationError);

	function beginEnrollment(): void {
		localError = '';
		validationError = '';
		notice = '';
		setupMutation.mutate(undefined);
	}

	function confirmEnrollment(event: SubmitEvent): void {
		event.preventDefault();
		if (!(event.currentTarget instanceof HTMLFormElement)) return;
		const form = event.currentTarget;
		const code = String(new FormData(form).get('totp-code') ?? '').trim();
		const result = v.safeParse(totpCodeSchema, code);

		if (!result.success) {
			validationError = result.issues[0]?.message ?? 'Enter a valid six-digit code.';

			return;
		}
		validationError = '';
		localError = '';
		notice = '';
		confirmMutation.mutate(code, { onSuccess: () => form.reset() });
	}

	function disableTOTP(event: SubmitEvent): void {
		event.preventDefault();
		if (!(event.currentTarget instanceof HTMLFormElement)) return;
		const form = event.currentTarget;
		const value = String(new FormData(form).get('totp-proof') ?? '').trim();
		const result = v.safeParse(totpProofSchema, value);

		if (!result.success) {
			validationError =
				result.issues[0]?.message ?? 'Enter a valid authenticator or recovery code.';

			return;
		}
		validationError = '';
		localError = '';
		notice = '';
		const proof = v.safeParse(totpCodeSchema, value).success
			? { code: value }
			: { recoveryCode: value };
		disableMutation.mutate(proof, { onSuccess: () => form.reset() });
	}

	async function copy(value: string, label: string): Promise<void> {
		localError = '';
		try {
			await navigator.clipboard.writeText(value);
			notice = `${label} copied.`;
		} catch {
			localError = `Could not copy ${label.toLowerCase()}.`;
		}
	}
</script>

<section class="border-b border-(--border-soft) py-5" aria-labelledby="totp-heading">
	<div class="flex items-start gap-3">
		<ShieldCheck class="mt-0.5 size-5 shrink-0 text-(--accent-cool)" aria-hidden="true" />
		<div class="min-w-0 flex-1">
			<h3 id="totp-heading" class="m-0 text-sm leading-5 font-medium text-(--text)">
				Authenticator app
			</h3>
			<p class="mt-1 mb-4 text-sm leading-5 text-(--muted)">
				Use an authenticator app to require a time-based code after your password. Arkive stores the
				secret encrypted on this server.
			</p>

			{#if statusQuery.isPending}
				<p class="m-0 text-sm text-(--muted)" role="status">Loading two-factor settings…</p>
			{:else if statusQuery.isError}
				<p class="m-0 text-sm text-(--danger)" role="alert">{statusQuery.error.message}</p>
				<Button
					text="Retry"
					variant="secondary"
					class="mt-3"
					onclick={() => statusQuery.refetch()}
				/>
			{:else if statusQuery.data.enabled}
				<p class="m-0 text-sm text-(--text)" role="status">
					Two-factor authentication is on. {statusQuery.data.recoveryCodesRemaining} recovery codes remain.
				</p>
				<form class="mt-4 max-w-md" onsubmit={disableTOTP}>
					<InputField
						label="Authenticator code or recovery code"
						name="totp-proof"
						placeholder="6-digit code or recovery code"
						autocomplete="one-time-code"
						required
						fieldClass="mb-3!"
						hasError={!!validationError}
						helperText={validationError}
						oninput={() => (validationError = '')}
					/>
					<Button
						text="Turn off two-factor authentication"
						variant="secondary"
						busy={disableMutation.isPending}
						busyText="Turning off…"
						type="submit"
					/>
				</form>
			{:else if setup}
				<div class="grid max-w-md gap-3">
					<p class="m-0 text-sm leading-5 text-(--text)">
						Scan this QR code with your authenticator app, then enter the current six-digit code.
					</p>
					<img
						class="size-48 rounded border border-(--border) bg-white p-2"
						src={setup.qrCodeDataURL}
						alt="Authenticator setup QR code"
					/>
					<div>
						<p class="m-0 text-xs text-(--muted)">Can’t scan it? Enter this setup key manually:</p>
						<code class="mt-1 block break-all font-mono text-sm text-(--text)">{setup.secret}</code>
					</div>
					<form onsubmit={confirmEnrollment}>
						<InputField
							label="Verification code"
							name="totp-code"
							placeholder="6-digit code"
							inputmode="numeric"
							autocomplete="one-time-code"
							maxlength={6}
							required
							fieldClass="mb-3!"
							hasError={!!validationError}
							helperText={validationError}
							oninput={() => (validationError = '')}
						/>
						<Button
							text="Verify and turn on"
							variant="primary"
							type="submit"
							busy={confirmMutation.isPending}
							busyText="Verifying…"
						/>
					</form>
				</div>
			{:else}
				<Button
					text="Set up authenticator app"
					variant="secondary"
					busy={setupMutation.isPending}
					busyText="Preparing…"
					onclick={beginEnrollment}
				/>
			{/if}

			{#if recoveryCodes.length > 0}
				<div class="mt-4 rounded-md border border-(--border) p-4">
					<p class="m-0 text-sm leading-5 font-medium text-(--text)">Save your recovery codes</p>
					<p class="mt-1 mb-3 text-sm leading-5 text-(--muted)">
						Each code works once if you lose access to your authenticator. Arkive cannot show them
						again.
					</p>
					<ul
						class="m-0 grid list-none grid-cols-1 gap-1 p-0 font-mono text-sm text-(--text) sm:grid-cols-2"
					>
						{#each recoveryCodes as code (code)}<li>{code}</li>{/each}
					</ul>
					<Button
						text="Copy recovery codes"
						variant="secondary"
						class="mt-3"
						onclick={() => copy(recoveryCodes.join('\n'), 'Recovery codes')}
					>
						{#snippet icon()}<Copy class="size-4" aria-hidden="true" />{/snippet}
					</Button>
					<Button
						text="I’ve saved them"
						variant="secondary"
						class="ml-2 mt-3"
						onclick={() => (recoveryCodes = [])}
					/>
				</div>
			{/if}

			{#if error}<p class="mt-3 mb-0 text-sm text-(--danger)" role="alert">{error}</p>{/if}
			{#if notice}<p class="mt-3 mb-0 text-sm text-(--muted)" role="status">{notice}</p>{/if}
		</div>
	</div>
</section>
