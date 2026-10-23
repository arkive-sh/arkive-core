<script lang="ts">
	import { goto } from '$app/navigation';
	import { createMutation, useQueryClient } from '@tanstack/svelte-query';
	import { Key, Lock, Shield } from '@lucide/svelte';
	import { loginAndUnlock } from '#lib/auth.ts';
	import Button from '#lib/components/Button.svelte';
	import Card from '#lib/components/Card.svelte';
	import InputField from '#lib/components/InputField.svelte';

	const queryClient = useQueryClient();
	const loginMutation = createMutation(() => ({
		mutationFn: ({ email, password }: { email: string; password: string }) => loginAndUnlock(email, password)
	}));
	let generalError = $state('');
	let submitting = $derived(loginMutation.isPending);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (loginMutation.isPending) return;

		const form = event.currentTarget as HTMLFormElement;
		const passwordInput = form.elements.namedItem('password') as HTMLInputElement;
		const email = String(new FormData(form).get('email') || '').trim();
		const password = passwordInput.value;
		generalError = '';

		try {
			queryClient.clear();
			await loginMutation.mutateAsync({ email, password });
			passwordInput.value = '';
			await goto('/', { replace: true });
		} catch (cause) {
			generalError = cause instanceof Error ? cause.message : 'Unlock failed. Try again.';
		}
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
					<Lock class="size-8 stroke-2" aria-hidden="true" />
				</span>
				<h1 class="m-0 text-2xl leading-8 font-semibold">Arkive Core</h1>
				<p class="m-0 text-[13px] leading-4.5 text-(--muted)">Sign in to your self-hosted encrypted file server.</p>
			</header>

			<Card class="w-full! rounded-lg! border-[color-mix(in_srgb,var(--border)_88%,white_8%)]! bg-[color-mix(in_srgb,var(--surface)_88%,black)]! p-7.5! shadow-[0_10px_30px_rgba(0,0,0,0.32)]!">
				<form class="grid gap-4" method="POST" onsubmit={submit}>
					{#if generalError}
						<p class="m-0 rounded border border-[rgba(147,0,10,0.3)] bg-[rgba(147,0,10,0.18)] px-3.5 py-3 text-[13px] leading-4.5 text-(--danger)" role="alert">{generalError}</p>
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
					<p class="m-0 rounded bg-[rgba(107,209,230,0.12)] px-3.5 py-3 text-[13px] leading-4.5 text-(--accent-cool)">
						<a href="/forgot-password" class="underline decoration-current underline-offset-2">Forgot password?</a>
					</p>
				</form>
			</Card>

			<footer class="inline-flex items-center justify-center gap-2.5 font-mono text-[13px] leading-4.5 text-[color-mix(in_srgb,var(--muted)_82%,white_4%)]">
				<span class="size-1.5 rounded-full bg-(--accent-cool) shadow-[0_0_0_3px_rgba(107,209,230,0.08)]"></span>
				<span>System standing by.</span>
			</footer>
		</div>
	</div>
</div>
