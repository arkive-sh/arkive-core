<script lang="ts">
	import { Eye, EyeOff } from '@lucide/svelte';
	import type { Snippet } from 'svelte';

	type Props = {
		label: string;
		name: string;
		type?: 'text' | 'password' | 'email' | 'number';
		description?: string;
		placeholder?: string;
		value?: string;
		required?: boolean;
		helperText?: string;
		hasError?: boolean;
		inputClass?: string;
		suffix?: Snippet;
	};

	let {
		label,
		name,
		type = 'text',
		description = '',
		placeholder = '',
		value,
		required = false,
		helperText = '',
		hasError = false,
		inputClass = '',
		suffix
	}: Props = $props();

	let showPassword = $state(false);
	let inputType = $derived(type === 'password' && showPassword ? 'text' : type);
	let invalid = $derived(hasError || helperText !== '');
	let inputClasses = $derived(
		[
			'form-input',
			type === 'password' && 'password-input',
			inputClass,
			invalid && 'is-invalid'
		]
			.filter(Boolean)
			.join(' ')
	);
	let helperID = $derived(helperText ? `${name}-helper` : undefined);
</script>

<div class="form-field">
	<label class="form-label" for={name}>
		<span>{label}</span>
		{#if suffix}<span class="form-label-suffix">{@render suffix()}</span>{/if}
	</label>
	{#if description}<p class="form-subtitle">{description}</p>{/if}
	{#if type === 'password'}
		<div class="password-wrapper">
			<input
				class={inputClasses}
				id={name}
				{name}
				type={inputType}
				{placeholder}
				{value}
				{required}
				aria-invalid={invalid ? 'true' : undefined}
				aria-describedby={helperID}
			/>
			<button
				class="password-toggle"
				type="button"
				aria-label={showPassword ? 'Hide password' : 'Show password'}
				aria-pressed={showPassword}
				data-visible={showPassword ? 'true' : 'false'}
				onclick={() => (showPassword = !showPassword)}
			>
				<span class="icon-eye">
					<Eye class="input-lucide" aria-hidden="true" />
				</span>
				<span class="icon-eye-off">
					<EyeOff class="input-lucide" aria-hidden="true" />
				</span>
			</button>
		</div>
	{:else}
		<input
			class={inputClasses}
			id={name}
			{name}
			type={inputType}
			{placeholder}
			{value}
			{required}
			aria-invalid={invalid ? 'true' : undefined}
			aria-describedby={helperID}
		/>
	{/if}
	{#if helperText}<p class="form-helper" id={helperID}>{helperText}</p>{/if}
</div>
