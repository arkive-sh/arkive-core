<script lang="ts">
	import { Eye, EyeOff } from '@lucide/svelte';
	import type { HTMLInputAttributes } from 'svelte/elements';
	import type { Snippet } from 'svelte';

	type Props = Omit<HTMLInputAttributes, 'class' | 'id' | 'name' | 'type' | 'value'> & {
		label: string;
		name: string;
		type?: HTMLInputAttributes['type'];
		id?: string;
		class?: string;
		value?: string;
		description?: string;
		helperText?: string;
		hasError?: boolean;
		fieldClass?: string;
		labelClass?: string;
		inputClass?: string;
		suffix?: Snippet;
	};

	let {
		label,
		name,
		type = 'text',
		id = name,
		class: nativeClass = '',
		description = '',
		placeholder = '',
		value,
		required = false,
		helperText = '',
		hasError = false,
		fieldClass = '',
		labelClass = '',
		inputClass = '',
		suffix,
		...inputProps
	}: Props = $props();

	let showPassword = $state(false);
	let inputType = $derived(type === 'password' && showPassword ? 'text' : type);
	let invalid = $derived(hasError || helperText !== '');
	let inputClasses = $derived(
		[
			'min-h-10 w-full rounded-md border border-(--border) bg-(--surface) px-3 py-2.5 text-sm leading-5 text-(--text) outline-none [transition:border-color_0.2s_ease,box-shadow_0.2s_ease] placeholder:text-(--muted) focus:border-(--accent)',
			type === 'password' && 'pr-13.5',
			nativeClass,
			inputClass,
			invalid && 'border-(--danger)!'
		]
			.filter(Boolean)
			.join(' ')
	);
	let helperID = $derived(helperText ? `${id}-helper` : undefined);
	let describedBy = $derived([inputProps['aria-describedby'], helperID].filter(Boolean).join(' ') || undefined);
</script>

<div class={`mb-4 flex flex-col gap-2 ${fieldClass}`}>
	<label class={`flex items-center justify-between gap-2 text-[11px] font-semibold uppercase leading-4 tracking-wider text-(--muted) ${labelClass}`} for={id}>
		<span>{label}</span>
		{#if suffix}<span class="inline-flex items-center text-(--accent-cool)">{@render suffix()}</span>{/if}
	</label>
	{#if description}<p class="m-0 text-[13px] leading-4.5 text-(--muted)">{description}</p>{/if}
	{#if type === 'password'}
		<div class="relative">
			<input
				{...inputProps}
				class={inputClasses}
				{id}
				{name}
				type={inputType}
				{placeholder}
				{value}
				{required}
				aria-invalid={invalid ? 'true' : inputProps['aria-invalid']}
				aria-describedby={describedBy}
			/>
			<button
				class="absolute right-2 top-1/2 inline-flex -translate-y-1/2 cursor-pointer items-center justify-center rounded bg-transparent p-1.5 text-(--muted) hover:text-(--text)"
				type="button"
				aria-label={showPassword ? 'Hide password' : 'Show password'}
				aria-pressed={showPassword}
				onclick={() => (showPassword = !showPassword)}
			>
				{#if showPassword}
					<EyeOff class="size-5" aria-hidden="true" />
				{:else}
					<Eye class="size-5" aria-hidden="true" />
				{/if}
			</button>
		</div>
	{:else}
		<input
			{...inputProps}
			class={inputClasses}
			{id}
			{name}
			type={inputType}
			{placeholder}
			{value}
			{required}
			aria-invalid={invalid ? 'true' : inputProps['aria-invalid']}
			aria-describedby={describedBy}
		/>
	{/if}
	{#if helperText}<p class="m-0 text-[13px] leading-4.5 text-(--danger)" id={helperID}>{helperText}</p>{/if}
</div>
