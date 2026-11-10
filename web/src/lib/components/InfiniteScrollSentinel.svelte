<script lang="ts">
	let {
		enabled,
		onLoad,
		rootSelector,
		rootMargin = '400px',
	}: {
		enabled: boolean;
		onLoad: () => void;
		rootSelector?: string;
		rootMargin?: string;
	} = $props();
	let sentinel = $state<HTMLDivElement>();

	$effect(() => {
		const target = sentinel;

		if (!enabled || !target || typeof IntersectionObserver === 'undefined') return;

		const observer = new IntersectionObserver(
			([entry]) => {
				if (entry?.isIntersecting) onLoad();
			},
			{ root: rootSelector ? target.closest(rootSelector) : null, rootMargin },
		);
		observer.observe(target);

		return () => observer.disconnect();
	});
</script>

<div bind:this={sentinel} class="h-1" aria-hidden="true"></div>
