<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import axios from 'axios';
	import { Copy, Download, Printer, ShieldCheck, TriangleAlert } from '@lucide/svelte';
	import Button from '#lib/components/Button.svelte';
	import { api, type APIErrorResponse } from '#lib/api.ts';
	import { toast } from '#lib/toast.ts';
	import { callVaultWorker } from '#lib/vault/worker-client.ts';
	import { fromBase64 } from '#lib/vault/bytes.ts';
	import { SETUP_MASTER_KEY_STORAGE, type InitialRecoveryRequest } from '../types';
	import { downloadRecoveryPDF } from './recovery-pdf';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let recoveryKey = $state('');
	let encryptedMasterKeyRecovery = $state('');
	let recoveryProofPublicKey = $state('');
	let acknowledged = $state(false);
	let busy = $state(false);
	let errorMessage = $state('');
	let generating = $state(true);
	const words = $derived(recoveryKey ? recoveryKey.split('-') : []);
	const canSubmit = $derived(Boolean(recoveryKey && encryptedMasterKeyRecovery && recoveryProofPublicKey && acknowledged && !busy));

	onMount(() => {
		let current = true;
		let storedMasterKey = sessionStorage.getItem(SETUP_MASTER_KEY_STORAGE);
		if (!storedMasterKey) {
			generating = false;
			errorMessage = 'Setup master key is unavailable in this tab. Return to the browser tab where setup started.';
			return;
		}

		let masterKey: Uint8Array<ArrayBuffer> | null = null;
		try {
			masterKey = fromBase64(storedMasterKey);
			storedMasterKey = null;
		} catch {
			generating = false;
			errorMessage = 'Setup master key is invalid. Return to the setup page and try again.';
			return;
		}

		const masterKeyBuffer = masterKey.buffer;
		void callVaultWorker('prepareInitialRecovery', { masterKeyBytes: masterKeyBuffer, userID: data.userID }, [masterKeyBuffer])
			.then((result) => {
				if (!current) return;
				recoveryKey = result.recoveryKey;
				encryptedMasterKeyRecovery = result.encryptedMasterKeyRecovery;
				recoveryProofPublicKey = result.recoveryProofPublicKey;
			})
			.catch((error: unknown) => {
				if (current) errorMessage = error instanceof Error ? error.message : 'Recovery key generation failed. Reload the page and try again.';
			})
			.finally(() => {
				if (masterKey?.byteLength) masterKey.fill(0);
				masterKey = null;
				if (current) generating = false;
			});

		return () => {
			current = false;
		};
	});

	async function copyRecoveryKey(): Promise<void> {
		try {
			await navigator.clipboard.writeText(recoveryKey);
			toast.success('Recovery key copied.');
		} catch {
			toast.error('Could not copy the recovery key.');
		}
	}

	function saveRecoveryPDF(): void {
		if (recoveryKey) downloadRecoveryPDF(recoveryKey);
	}

	function handleError(error: unknown): void {
		if (axios.isAxiosError<APIErrorResponse>(error)) {
			errorMessage = error.response?.data?.error?.details?.fields?.general ?? error.response?.data?.error?.message ?? error.message;
			return;
		}
		errorMessage = error instanceof Error ? error.message : 'Recovery key setup failed. Reload the page and try again.';
	}

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (!canSubmit) return;
		busy = true;
		errorMessage = '';
		try {
			const request: InitialRecoveryRequest = {
				confirmBackup: acknowledged,
				encryptedMasterKeyRecovery,
				recoveryProofPublicKey
			};
			await api.post('/api/setup/recovery-key', request);
			sessionStorage.removeItem(SETUP_MASTER_KEY_STORAGE);
			recoveryKey = '';
			encryptedMasterKeyRecovery = '';
			recoveryProofPublicKey = '';
			await goto('/login', { replace: true });
		} catch (error) {
			if (axios.isAxiosError(error) && error.response?.status === 401) {
				await goto('/login', { replace: true });
				return;
			}
			handleError(error);
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head>
	<title>Arkive · Vault Recovery Key</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<main class="grid min-h-screen bg-[linear-gradient(180deg,rgba(9,14,18,0.98),rgba(15,20,24,0.98))] px-4 py-5 text-(--text) md:place-items-center print:bg-white print:text-black">
	<section class="mx-auto w-full max-w-200 border border-(--border) bg-(--surface) p-5 shadow-[0_24px_60px_rgba(0,0,0,0.34)] print:border-gray-300 print:bg-white print:p-0 print:shadow-none" aria-labelledby="recovery-heading">
		<header class="mb-4.5 flex items-start gap-4">
			<span class="inline-flex size-14 shrink-0 items-center justify-center border border-(--danger)/30 bg-(--danger)/10 text-(--danger) print:hidden">
				<TriangleAlert class="size-7" aria-hidden="true" />
			</span>
			<div class="min-w-0">
				<h1 id="recovery-heading" class="m-0 mb-1 text-[1.75rem] leading-[1.05] font-semibold">Vault Recovery Key</h1>
				<p class="m-0 max-w-160 text-[15px] leading-[1.4] text-(--muted) print:text-gray-700">This is the <strong class="text-(--danger) italic print:text-black">ONLY</strong> way to recover your data if you lose your password. Arkive Core is a zero-knowledge system; we cannot reset your access.</p>
				<p class="mt-2.5 mb-0 text-sm leading-5 text-amber-300 print:hidden">If you refresh this page before confirming, Arkive Core will generate a new recovery key. Only back up the key currently shown on this page.</p>
			</div>
		</header>

		<form class="flex flex-col gap-4" method="post" onsubmit={submit}>
			<div class="border border-(--border)/70 bg-(--surface-raised) p-5 print:border-gray-300 print:bg-white">
				<div class="mb-4.5 flex justify-end gap-2 print:hidden">
					{#snippet copyIcon()}<Copy class="size-4" aria-hidden="true" />{/snippet}
					<Button text="Copy key" variant="secondary" disabled={!recoveryKey} onclick={copyRecoveryKey} icon={copyIcon} />
					{#snippet printIcon()}<Printer class="size-4" aria-hidden="true" />{/snippet}
					<Button text="Print" variant="secondary" disabled={!recoveryKey} onclick={() => window.print()} icon={printIcon} />
				</div>
				{#if generating}
					<p class="m-0 text-sm text-(--muted)" role="status">Generating recovery key...</p>
				{:else if recoveryKey}
					<ol class="m-0 grid list-none grid-cols-1 gap-x-5 gap-y-3 p-0 font-mono sm:grid-cols-2 lg:grid-cols-4 print:grid-cols-4">
						{#each words as word, index (index)}
							<li class="flex min-w-0 items-baseline gap-3 border-b border-(--border)/40 pb-1.5 print:border-gray-300">
								<span class="w-6 shrink-0 text-[11px] text-(--muted-strong) print:text-gray-600">{String(index + 1).padStart(2, '0')}</span>
								<strong class="min-w-0 wrap-anywhere text-[15px] text-(--text) print:text-black">{word}</strong>
							</li>
						{/each}
					</ol>
				{/if}
			</div>

			{#if errorMessage}<p class="m-0 text-sm text-(--danger) print:hidden" role="alert">{errorMessage}</p>{/if}

			<label class="flex cursor-pointer items-start gap-4 print:hidden">
				<input class="mt-0.5 size-5.5 shrink-0 accent-(--accent)" type="checkbox" bind:checked={acknowledged} />
				<span class="text-[15px] leading-[1.35] text-(--muted)">I have written down or otherwise securely backed up this recovery key. I understand that Arkive Core cannot recover this key for me and losing it will result in <strong class="text-(--text)">permanent data loss</strong>.</span>
			</label>

			<footer class="flex flex-col gap-3 border-t border-(--border)/40 pt-4 sm:flex-row print:hidden">
				{#snippet submitIcon()}<ShieldCheck class="size-4" aria-hidden="true" />{/snippet}
				<Button type="submit" text="Confirm & Secure Vault" busyText="Securing..." variant="primary" busy={busy} disabled={!canSubmit} class="min-h-13 flex-1 text-xs font-extrabold tracking-[0.08em] uppercase" icon={submitIcon} />
				{#snippet downloadIcon()}<Download class="size-4" aria-hidden="true" />{/snippet}
				<Button text="Download Recovery PDF" variant="secondary" disabled={!recoveryKey} onclick={saveRecoveryPDF} class="min-h-13 flex-1 text-xs font-extrabold tracking-[0.08em] uppercase" icon={downloadIcon} />
			</footer>
		</form>
	</section>
</main>
