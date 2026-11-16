<script lang="ts">
	import InputField from '#lib/components/InputField.svelte';

	type Values = {
		endpoint?: string;
		bucket?: string;
		region?: string;
		accessKeyID?: string;
		usePathStyle?: boolean;
	};
	type Props = {
		values?: Values;
		errors?: Record<string, string>;
		required?: boolean;
		showCredentialStatus?: boolean;
		secretConfigured?: boolean;
		sessionTokenConfigured?: boolean;
		allowClearSessionToken?: boolean;
	};

	let {
		values = {},
		errors = {},
		required = false,
		showCredentialStatus = false,
		secretConfigured = false,
		sessionTokenConfigured = false,
		allowClearSessionToken = false,
	}: Props = $props();
</script>

<div class="grid grid-cols-2 gap-4 max-[760px]:grid-cols-1">
	<div class="col-span-2 max-[760px]:col-span-1">
		<InputField
			label="Endpoint URL"
			name="s3_endpoint"
			value={values.endpoint ?? ''}
			placeholder="https://s3.region.amazonaws.com"
			helperText={errors.s3_endpoint ?? ''}
			{required}
		/>
	</div>
	<InputField
		label="Bucket name"
		name="s3_bucket"
		value={values.bucket ?? ''}
		placeholder="arkive-vault-01"
		helperText={errors.s3_bucket ?? ''}
		{required}
	/>
	<InputField
		label="Region"
		name="s3_region"
		value={values.region ?? 'auto'}
		placeholder="us-east-1"
	/>
	<InputField
		label="Access key ID"
		name="s3_access_key_id"
		value={values.accessKeyID ?? ''}
		helperText={errors.s3_access_key_id ?? ''}
		{required}
	/>
	<div>
		<InputField
			label="Secret access key"
			name="s3_secret_access_key"
			type="password"
			placeholder={showCredentialStatus && secretConfigured ? 'Saved — leave blank to keep' : ''}
			description={showCredentialStatus
				? secretConfigured
					? 'A secret key is saved.'
					: 'No secret key is saved.'
				: ''}
			helperText={errors.s3_secret_access_key ?? ''}
		/>
	</div>
	<div>
		<InputField
			label="Session token"
			name="s3_session_token"
			type="password"
			placeholder={showCredentialStatus && sessionTokenConfigured
				? 'Saved — leave blank to keep'
				: showCredentialStatus
					? 'Optional'
					: ''}
		/>
		{#if allowClearSessionToken && sessionTokenConfigured}
			<label
				class="-mt-2 mb-4 inline-flex cursor-pointer items-center gap-2.5 text-sm text-(--muted)"
			>
				<input class="accent-(--accent)" type="checkbox" name="clear_session_token" />
				<span>Remove the saved session token</span>
			</label>
		{/if}
	</div>
</div>

<label
	class="mt-1 mb-4 inline-flex cursor-pointer items-center gap-2.5 text-[13px] leading-5 text-(--muted)"
>
	<input
		class="accent-(--accent)"
		type="checkbox"
		name="s3_use_path_style"
		checked={values.usePathStyle ?? false}
	/>
	<span>Use path-style addressing</span>
</label>
