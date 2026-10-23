<script lang="ts">
	import { Cloud, HardDrive } from '@lucide/svelte';

	type StorageProvider = 'local' | 's3';

	let { value, onchange }: { value: StorageProvider; onchange: (value: StorageProvider) => void } = $props();
</script>

<fieldset class="inline-flex w-fit flex-wrap gap-2 rounded border border-(--border) bg-(--surface-highest) p-1 max-[560px]:w-full">
	<legend class="sr-only">Storage provider</legend>
	<label class="cursor-pointer">
		<input class="peer sr-only" type="radio" name="storageProvider" value="local" checked={value === 'local'} onchange={() => onchange('local')} />
		<span class="inline-flex min-h-9 items-center gap-2 rounded border border-transparent px-3.5 text-(--muted) transition-colors peer-checked:border-(--accent) peer-checked:bg-(--surface) peer-checked:text-(--text) peer-focus-visible:ring-2 peer-focus-visible:ring-(--accent)">
			<HardDrive class="size-4 text-(--accent-strong)" aria-hidden="true" />
			<strong class="font-semibold">Local</strong>
		</span>
	</label>
	<label class="cursor-pointer">
		<input class="peer sr-only" type="radio" name="storageProvider" value="s3" checked={value === 's3'} onchange={() => onchange('s3')} />
		<span class="inline-flex min-h-9 items-center gap-2 rounded border border-transparent px-3.5 text-(--muted) transition-colors peer-checked:border-(--accent) peer-checked:bg-(--surface) peer-checked:text-(--text) peer-focus-visible:ring-2 peer-focus-visible:ring-(--accent)">
			<Cloud class="size-4 text-(--accent-strong)" aria-hidden="true" />
			<strong class="font-semibold">S3 Compatible</strong>
		</span>
	</label>
</fieldset>
