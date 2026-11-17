<script lang="ts">
	let { className = '' }: { className?: string } = $props();
</script>

<div
	class={`motion-safe:animate-pulse rounded bg-(--surface-highest) ${className}`}
	aria-hidden="true"
></div>
