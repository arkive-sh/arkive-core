<script lang="ts">
	import axios from 'axios';
	import { goto } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { api, type APIErrorResponse } from '#lib/api.ts';
	import Button from '#lib/components/Button.svelte';
	import Card from '#lib/components/Card.svelte';
	import InputField from '#lib/components/InputField.svelte';

	let error = $state('');
	let submitting = $state(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (submitting) return;
		const form = event.currentTarget;

		if (!(form instanceof HTMLFormElement)) return;
		const email = String(new FormData(form).get('email') ?? '').trim();
		error = '';
		submitting = true;
		try {
			const { data } = await api.post<{ resetURL: string }>('/api/auth/forgot-password', {
				email: email.trim(),
			});
			const token = new URL(data.resetURL, window.location.origin).searchParams.get('token');

			if (!token)
				throw new Error('No recovery link was generated. Check that a recovery key was set up.');
			await goto(resolve(`/reset-password?token=${encodeURIComponent(token)}`));
		} catch (cause) {
			error = axios.isAxiosError<APIErrorResponse>(cause)
				? (cause.response?.data?.error?.message ?? cause.message)
				: cause instanceof Error
					? cause.message
					: 'Password recovery could not be started.';
		} finally {
			submitting = false;
		}
	}
</script>

<svelte:head>
	<title>Arkive · Recover access</title>
	<meta name="robots" content="noindex, nofollow" />
</svelte:head>

<main class="flex min-h-screen items-center justify-center bg-(--bg) px-4 py-8 text-(--text)">
	<div class="grid w-[min(420px,92vw)] gap-6">
		<header class="text-center">
			<h1 class="m-0 text-2xl font-semibold">Recover access</h1>
			<p class="mt-2 text-sm text-(--muted)">Use your recovery key to set a new password.</p>
		</header>
		<Card class="w-full! rounded-lg! border-(--border-soft)! bg-(--surface-raised)! p-7!">
			<form class="grid gap-4" onsubmit={submit}>
				{#if error}<p
						class="m-0 rounded border border-(--danger) bg-(--danger)/10 px-3 py-2 text-sm text-(--danger)"
						role="alert"
					>
						{error}
					</p>{/if}
				<InputField
					label="Email"
					name="email"
					type="email"
					placeholder="user@local.node"
					required
				/>
				<Button
					text="Continue"
					type="submit"
					variant="primary"
					busy={submitting}
					busyText="Checking account..."
					class="w-full!"
				/>
				<a href={resolve('/login')} class="text-center text-sm text-(--muted) underline"
					>Back to login</a
				>
			</form>
		</Card>
	</div>
</main>
