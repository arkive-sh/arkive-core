<script lang="ts">
	import axios from 'axios';
	import { goto } from '$app/navigation';
	import { BadgeInfo, Database, LockKeyhole, Rocket, Server, ShieldAlert } from '@lucide/svelte';
	import Button from '#lib/components/Button.svelte';
	import InputField from '#lib/components/InputField.svelte';
	import SetupSection from '#lib/components/SetupSection.svelte';
	import StorageSelector from '#lib/components/StorageSelector.svelte';
	import { api, type APIErrorResponse } from '#lib/api.ts';
	import { opaqueAuth } from '#lib/opaque-auth.ts';
	import { callVaultWorker } from '#lib/vault/worker-client.ts';
	import { toBase64 } from '#lib/vault/bytes.ts';
	import {
		SETUP_MASTER_KEY_STORAGE,
		type InitialSetupRequest,
		type SetupStatus,
		type StorageProvider,
	} from './types';
	import { validateSetup, type SetupFormValues } from './validation';

	let storageProvider = $state<StorageProvider>('local');
	let submitting = $state(false);
	let generalError = $state('');
	let fieldErrors = $state<Record<string, string>>({});

	function readFormValues(values: FormData): SetupFormValues {
		const read = (name: string) => String(values.get(name) ?? '').trim();

		return {
			name: read('name'),
			username: read('username'),
			email: read('email'),
			password: read('password'),
			confirm_password: read('confirm_password'),
			storage_provider: storageProvider,
			storage_gb: read('storage_gb'),
			local_path: read('local_path'),
			s3_access_key_id: read('s3_access_key_id'),
			s3_secret_access_key: read('s3_secret_access_key'),
			s3_bucket: read('s3_bucket'),
			s3_endpoint: read('s3_endpoint'),
		};
	}

	function validateInput(event: Event): void {
		if (
			!(event.currentTarget instanceof HTMLFormElement) ||
			!(event.target instanceof HTMLInputElement)
		)
			return;

		const field = event.target.name;
		const result = validateSetup(readFormValues(new FormData(event.currentTarget)));
		const issues = result.success ? [] : result.issues;
		const fields =
			field === 'password' || field === 'confirm_password'
				? ['password', 'confirm_password']
				: [field];
		const nextErrors = { ...fieldErrors };

		for (const name of fields) {
			const issue = issues.find((issue) => issue.path?.[0]?.key === name);

			if (issue) nextErrors[name] = issue.message;
			else delete nextErrors[name];
		}
		fieldErrors = nextErrors;
		generalError = '';
	}

	function showError(error: unknown): void {
		if (axios.isAxiosError<APIErrorResponse>(error)) {
			fieldErrors = error.response?.data?.error?.details?.fields ?? {};
			const hasFieldErrors = Object.keys(fieldErrors).some((field) => field !== 'general');
			generalError = hasFieldErrors
				? ''
				: (fieldErrors.general ?? error.response?.data?.error?.message ?? error.message);

			return;
		}
		generalError = error instanceof Error ? error.message : 'Setup failed. Please try again.';
	}

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (!(event.currentTarget instanceof HTMLFormElement) || submitting) return;

		const form = event.currentTarget;
		const values = new FormData(form);
		const formValues = readFormValues(values);
		let password: string | null = formValues.password;
		generalError = '';
		fieldErrors = {};

		const validation = validateSetup(formValues);

		if (!validation.success) {
			for (const issue of validation.issues) {
				const field = issue.path?.[0]?.key;

				if (typeof field === 'string') fieldErrors[field] ??= issue.message;
			}
			if (!Object.keys(fieldErrors).length) generalError = 'Please correct the highlighted fields.';

			return;
		}

		submitting = true;

		let vault: {
			masterKey: Uint8Array<ArrayBuffer>;
			vaultSalt: string;
			encryptedMasterKey: string;
		} | null = null;
		let registration: Awaited<ReturnType<typeof opaqueAuth.registerSetup>> | null = null;
		let exportKey: Uint8Array<ArrayBuffer> | null = null;
		let registrationRecord = '';

		try {
			if (password === null) throw new Error('Password is unavailable.');
			registration = await opaqueAuth.registerSetup(formValues.username, password);
			password = null;
			formValues.password = '';
			formValues.confirm_password = '';
			values.set('password', '');
			values.set('confirm_password', '');
			registrationRecord = registration.registrationRecord;
			exportKey = registration.exportKey;
			registration = null;
			if (!exportKey) throw new Error('Vault export key is unavailable.');
			try {
				vault = await callVaultWorker('createInitialVault', { exportKey: exportKey.buffer }, [
					exportKey.buffer,
				]);
			} finally {
				if (exportKey.byteLength) exportKey.fill(0);
				exportKey = null;
			}
			const vaultSalt = vault.vaultSalt;
			const encryptedMasterKey = vault.encryptedMasterKey;
			sessionStorage.setItem(SETUP_MASTER_KEY_STORAGE, toBase64(vault.masterKey));
			vault.masterKey.fill(0);
			vault = null;
			const request: InitialSetupRequest = {
				name: formValues.name,
				username: formValues.username,
				email: formValues.email,
				opaqueRegistrationRecord: registrationRecord,
				vaultSalt,
				encryptedMasterKey,
				storage: {
					provider: storageProvider,
					localPath: storageProvider === 'local' ? formValues.local_path : '',
					storageGB: formValues.storage_gb,
					s3AccessKeyID: storageProvider === 's3' ? formValues.s3_access_key_id : '',
					s3SecretAccessKey: storageProvider === 's3' ? formValues.s3_secret_access_key : '',
					s3SessionToken:
						storageProvider === 's3' ? String(values.get('s3_session_token') ?? '').trim() : '',
					s3Bucket: storageProvider === 's3' ? formValues.s3_bucket : '',
					s3Endpoint: storageProvider === 's3' ? formValues.s3_endpoint : '',
					s3Region: storageProvider === 's3' ? String(values.get('s3_region') ?? '').trim() : '',
					s3UsePathStyle: storageProvider === 's3' && values.get('s3_use_path_style') === 'on',
				},
			};
			await api.post('/api/setup', request);
			await goto('/setup/recovery-key', { replace: true });
		} catch (error) {
			if (axios.isAxiosError(error) && error.response?.status === 409) {
				try {
					const { data: status } = await api.get<SetupStatus>('/api/setup/status');

					if (status.recoveryPending && sessionStorage.getItem(SETUP_MASTER_KEY_STORAGE)) {
						await goto('/setup/recovery-key', { replace: true });

						return;
					}
				} catch {
					// Keep the local key when setup may have completed despite a lost response.
				}
				sessionStorage.removeItem(SETUP_MASTER_KEY_STORAGE);
				await goto('/login', { replace: true });

				return;
			}
			if (axios.isAxiosError(error) && error.response && error.response.status < 500) {
				sessionStorage.removeItem(SETUP_MASTER_KEY_STORAGE);
			}
			showError(error);
		} finally {
			password = null;
			formValues.password = '';
			formValues.confirm_password = '';
			values.set('password', '');
			values.set('confirm_password', '');
			for (const input of form.querySelectorAll<HTMLInputElement>('input[type="password"]'))
				input.value = '';
			if (exportKey?.byteLength) exportKey.fill(0);
			exportKey = null;
			registration = null;
			vault?.masterKey.fill(0);
			vault = null;
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Arkive · Setup</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<main class="min-h-screen bg-(--bg) px-4 py-8 text-(--text) max-[760px]:px-3 max-[760px]:py-4">
	<div class="mx-auto w-[min(920px,100%)]">
		<form
			class="overflow-hidden rounded-md border border-(--border-soft) bg-(--surface-raised) shadow-[0_10px_28px_rgba(0,0,0,0.28)]"
			method="post"
			novalidate
			onsubmit={submit}
			oninput={validateInput}
		>
			<header
				class="flex items-center gap-3 border-b border-(--border-soft) px-6 py-4.5 max-[760px]:px-4"
			>
				<span class="inline-flex items-center justify-center text-(--accent)"
					><Server class="size-5" aria-hidden="true" /></span
				>
				<div>
					<h1 class="m-0 text-xl leading-7 font-semibold">Initialize Arkive Core</h1>
					<p class="m-0 mt-1 text-[13px] leading-5 text-(--muted)">
						Configure your self-hosted encrypted file server.
					</p>
				</div>
			</header>

			<div class="px-6 max-[760px]:px-4">
				{#if generalError}
					<p
						class="mt-4 mb-0 rounded border border-(--danger)/30 bg-(--danger)/10 px-3 py-2.5 text-sm text-(--danger)"
						role="alert"
					>
						{generalError}
					</p>
				{/if}

				{#snippet instanceIcon()}<BadgeInfo class="size-4" aria-hidden="true" />{/snippet}
				<SetupSection number="1" title="Account Details" icon={instanceIcon}>
					<div class="grid grid-cols-2 gap-4 max-[760px]:grid-cols-1">
						<InputField
							label="Name"
							name="name"
							placeholder="Enter your name"
							required
							helperText={fieldErrors.name}
						/>
						<InputField
							label="Username"
							name="username"
							placeholder="Enter username"
							required
							helperText={fieldErrors.username}
						/>
						<InputField
							label="Admin email"
							name="email"
							type="email"
							placeholder="admin@domain.local"
							required
							helperText={fieldErrors.email}
						/>
					</div>
				</SetupSection>

				{#snippet securityIcon()}<LockKeyhole class="size-4" aria-hidden="true" />{/snippet}
				<SetupSection number="2" title="Security" icon={securityIcon}>
					<div
						class="flex items-start gap-3 rounded border border-(--border) bg-(--surface-highest) px-3.5 py-3"
					>
						<ShieldAlert class="mt-0.5 size-4.5 shrink-0 text-amber-400" aria-hidden="true" />
						<p class="m-0 text-[13px] leading-5 text-(--muted)">
							Arkive Core uses zero-knowledge encryption. This password derives your master
							encryption key. <strong class="text-(--danger)"
								>It cannot be recovered if lost.</strong
							>
						</p>
					</div>
					<div class="grid grid-cols-2 gap-4 max-[760px]:grid-cols-1">
						<InputField
							label="Master password"
							name="password"
							type="password"
							placeholder="Create a password"
							required
							helperText={fieldErrors.password}
						/>
						<InputField
							label="Confirm password"
							name="confirm_password"
							type="password"
							placeholder="Confirm your password"
							required
							helperText={fieldErrors.confirm_password}
						/>
					</div>
				</SetupSection>

				{#snippet storageIcon()}<Database class="size-4" aria-hidden="true" />{/snippet}
				<SetupSection number="3" title="Storage" icon={storageIcon}>
					<div class="grid gap-4">
						<StorageSelector
							value={storageProvider}
							onchange={(provider) => {
								storageProvider = provider;
								fieldErrors = {};
								generalError = '';
							}}
						/>
						<InputField
							label="Storage limit in GB"
							name="storage_gb"
							type="number"
							placeholder="0"
							value="0"
							required
							description="Use 0 for unlimited."
							min="0"
							helperText={fieldErrors.storage_gb}
						/>
						<div class="rounded border border-(--border-soft) bg-(--surface) p-3.5">
							<div class:hidden={storageProvider !== 'local'}>
								<InputField
									label="Storage path"
									name="local_path"
									placeholder="/var/lib/arkive/files"
									value="./data/storage"
									helperText={fieldErrors.local_path}
								/>
							</div>
							<div class:hidden={storageProvider !== 's3'}>
								<div class="grid grid-cols-2 gap-4 max-[760px]:grid-cols-1">
									<div class="col-span-2 max-[760px]:col-span-1">
										<InputField
											label="Endpoint URL"
											name="s3_endpoint"
											placeholder="https://s3.region.amazonaws.com"
											helperText={fieldErrors.s3_endpoint}
										/>
									</div>
									<InputField
										label="Bucket name"
										name="s3_bucket"
										placeholder="arkive-vault-01"
										helperText={fieldErrors.s3_bucket}
									/>
									<InputField
										label="Region"
										name="s3_region"
										placeholder="us-east-1"
										value="auto"
									/>
									<InputField
										label="Access key"
										name="s3_access_key_id"
										helperText={fieldErrors.s3_access_key_id}
									/>
									<InputField
										label="Secret key"
										name="s3_secret_access_key"
										type="password"
										helperText={fieldErrors.s3_secret_access_key}
									/>
									<InputField label="Session token" name="s3_session_token" type="password" />
								</div>
								<label
									class="mt-1 inline-flex cursor-pointer items-center gap-2.5 text-[13px] leading-5 text-(--muted)"
								>
									<input class="accent-(--accent)" type="checkbox" name="s3_use_path_style" />
									<span>Use path-style addressing</span>
								</label>
							</div>
						</div>
					</div>
				</SetupSection>
			</div>

			{#snippet submitIcon()}<Rocket class="size-4" aria-hidden="true" />{/snippet}
			<footer
				class="flex justify-end gap-3 border-t border-(--border-soft) bg-(--surface) px-6 py-4 max-[760px]:flex-col-reverse max-[760px]:px-4"
			>
				<Button
					type="reset"
					text="Cancel"
					variant="secondary"
					class="max-[760px]:w-full"
					onclick={() => {
						storageProvider = 'local';
						fieldErrors = {};
						generalError = '';
					}}
				/>
				<Button
					type="submit"
					text="Initialize Arkive Core"
					busyText="Initializing..."
					variant="primary"
					busy={submitting}
					class="max-[760px]:w-full"
					icon={submitIcon}
				/>
			</footer>
		</form>
	</div>
</main>
