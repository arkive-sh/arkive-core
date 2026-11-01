<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { LockKeyhole } from '@lucide/svelte';
	import Button from '#lib/components/Button.svelte';
	import InputField from '#lib/components/InputField.svelte';
	import { loginAndUnlock } from '#lib/auth.ts';
	import { vaultUnlocked } from '#lib/vault/state.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let password = $state('');
	let error = $state('');
	let submitting = $state(false);

	onMount(() => {
		if ($vaultUnlocked) void goto('/', { replaceState: true });
	});

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (submitting || !password) return;
		submitting = true;
		error = '';
		try {
			await loginAndUnlock(data.user.email, password);
			password = '';
			await goto('/', { replaceState: true });
		} catch (cause) {
			error = cause instanceof Error ? cause.message : 'Could not unlock the vault.';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Unlock vault | Arkive</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<section class="mx-auto grid min-h-[calc(100vh-56px)] max-w-md content-center px-4 py-10">
	<div class="rounded-lg border border-(--border) bg-(--surface-raised) p-6 min-[640px]:p-8">
		<div class="mb-5 flex size-12 items-center justify-center rounded-md bg-(--surface-highest) text-(--accent-cool)">
			<LockKeyhole class="size-6" aria-hidden="true" />
		</div>
		<h1 class="m-0 text-2xl leading-8 font-semibold text-(--text)">Unlock your vault</h1>
		<p class="mt-2 mb-6 text-sm leading-5 text-(--muted)">Enter your password to unlock your files on this device.</p>
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
			<Button text="Unlock vault" busyText="Unlocking…" type="submit" busy={submitting} disabled={!password} class="mt-2 w-full" />
		</form>
	</div>
</section>
