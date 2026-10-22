<script lang="ts">
	import { BadgeCheck, Search, Unlink } from '@lucide/svelte';
	import Button from '#lib/components/Button.svelte';

	let { status }: { status: number } = $props();
	let notFound = $derived(status === 404);
</script>

<main class="relative h-screen min-h-screen overflow-hidden bg-[#0a0f12] text-[#dee3e8]">
	<div class="pointer-events-none absolute inset-0 opacity-10 bg-[linear-gradient(#30363d_1px,transparent_1px),linear-gradient(90deg,#30363d_1px,transparent_1px)] bg-size-[40px_40px]"></div>
	<div class="relative z-1 box-border flex h-screen min-h-screen flex-col items-center justify-center p-6 max-[760px]:justify-start max-[760px]:pt-8 max-[640px]:px-4">
		<div class="mb-7 grid justify-items-center gap-3 max-[760px]:mb-5">
			<div class="relative flex size-32 items-center justify-center rounded-lg border border-[#3e484f] bg-[#171c20]">
				<Unlink class="size-16 stroke-[1.7] text-[#8ed5ff]" aria-hidden="true" />
				<div class="absolute -top-0.5 -left-0.5 size-2.5 border-t border-l border-[#8ed5ff]"></div>
				<div class="absolute -top-0.5 -right-0.5 size-2.5 border-t border-r border-[#8ed5ff]"></div>
				<div class="absolute -bottom-0.5 -left-0.5 size-2.5 border-b border-l border-[#8ed5ff]"></div>
				<div class="absolute -right-0.5 -bottom-0.5 size-2.5 border-r border-b border-[#8ed5ff]"></div>
			</div>
			<div class="inline-flex items-center gap-1.5">
				<span class="size-1 rounded-full bg-[#ffb4ab]"></span>
				<span class="font-mono text-[13px] leading-4.5 text-[#87929a]">{notFound ? 'ERROR_CODE: PATH_UNRESOLVED' : `ERROR_CODE: HTTP_${status}`}</span>
			</div>
		</div>
		<section class="grid w-full max-w-155 justify-items-center text-center">
			<h1 class="m-0 mb-4 text-2xl leading-8 font-semibold tracking-[-0.02em] text-[#dee3e8]">{notFound ? '404: Node Path Not Found' : `${status}: Request Failed`}</h1>
			<p class="m-0 max-w-105 text-sm leading-[1.6] text-[#bdc8d1]">
				{notFound
					? 'The requested resource does not exist on this node or has been moved. The address may be invalid or no longer available.'
					: 'The request could not be completed. Please try again or return to the dashboard.'}
			</p>
			<div class="mt-8 flex w-full max-w-130 justify-center gap-6 max-[760px]:mt-6 max-[640px]:flex-col max-[640px]:gap-3">
				<Button text="Return to Dashboard" href="/" class="min-h-13 min-w-45 rounded-lg px-5 text-[11px] leading-4 font-semibold tracking-wider text-[#001e2c] uppercase shadow-none border-[#38bdf8] bg-[#38bdf8] hover:border-[#69cffb] hover:bg-[#69cffb] max-[640px]:w-full max-[640px]:min-w-0" />
				<Button text="Open Files" href="/files" class="min-h-13 min-w-45 rounded-lg px-5 text-[11px] leading-4 font-semibold tracking-wider text-[#bdc8d1] uppercase shadow-none border-[#3e484f] bg-transparent hover:bg-[#252b2e] hover:text-[#dee3e8] max-[640px]:w-full max-[640px]:min-w-0">
					{#snippet icon()}<Search size="18" aria-hidden="true" />{/snippet}
				</Button>
			</div>
			<div class="mt-7 flex items-center justify-center gap-1.5 max-[760px]:mt-5">
				<p class="m-0 text-[13px] leading-4.5 text-[#87929a]">Powered by <span class="font-semibold text-[#dee3e8]">Arkive Core</span></p>
				<BadgeCheck class="size-3.5 stroke-2 text-[#87929a]" aria-hidden="true" />
			</div>
		</section>
	</div>
</main>
