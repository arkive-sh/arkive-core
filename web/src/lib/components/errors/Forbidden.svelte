<script lang="ts">
	import { ArrowLeft, BadgeCheck, Lock, Shield } from '@lucide/svelte';
	import Button from '#lib/components/Button.svelte';
</script>

<div class="relative h-screen min-h-screen overflow-hidden bg-[#0a0f12] text-[#dee3e8]">
	<div class="pointer-events-none fixed inset-0 opacity-10 bg-[linear-gradient(#30363d_1px,transparent_1px),linear-gradient(90deg,#30363d_1px,transparent_1px)] bg-size-[40px_40px]"></div>
	<div class="pointer-events-none fixed inset-0 z-2 opacity-[0.03] [background:linear-gradient(rgba(18,16,16,0)_50%,rgba(0,0,0,0.25)_50%),linear-gradient(90deg,rgba(255,0,0,0.06),rgba(0,255,0,0.02),rgba(0,0,255,0.06))] bg-size-[100%_2px,3px_100%]"></div>
	<main class="relative z-3 box-border flex h-screen min-h-screen flex-col items-center justify-center p-6 max-[760px]:justify-start max-[760px]:pt-6 max-[640px]:px-4">
		<div class="relative mb-7 max-[760px]:mb-5">
			<div class="absolute inset-0 scale-[1.45] rounded-full bg-[rgba(255,180,171,0.1)] opacity-[0.55] blur-[36px]"></div>
			<div class="relative flex size-24 items-center justify-center rounded-lg border border-[#3e484f] bg-[#252b2e] shadow-[0_24px_48px_rgba(0,0,0,0.35)]">
				<Lock class="size-14.5 fill-current stroke-[#ffb4ab] stroke-2 text-[#ffb4ab]" aria-hidden="true" />
				<div class="absolute -top-0.5 -left-0.5 size-4 border-t-2 border-l-2 border-[#38bdf8]"></div>
				<div class="absolute -right-0.5 -bottom-0.5 size-4 border-r-2 border-b-2 border-[#38bdf8]"></div>
			</div>
		</div>
		<section class="grid w-full max-w-125 justify-items-center text-center">
			<p class="mb-1 text-[11px] leading-4 font-semibold tracking-wider text-[#38bdf8] uppercase">Error Code: 403_FORBIDDEN</p>
			<h1 class="m-0 text-2xl leading-8 font-semibold tracking-[-0.02em] text-[#dee3e8]">403: Access Denied</h1>
			<div class="my-4 h-px w-12 bg-[#3e484f]"></div>
			<p class="m-0 max-w-80 text-sm leading-5 text-[#bdc8d1]">You do not have permission to access this resource.</p>
			<div class="mt-6 grid w-full max-w-70 gap-2">
				<Button text="Files" href="/" class="min-h-12 rounded-lg text-[11px] leading-4 font-semibold tracking-wider text-[#001e2c] uppercase shadow-none border-[#38bdf8] bg-[#38bdf8] hover:border-[#69cffb] hover:bg-[#69cffb]">
					{#snippet icon()}<Shield size="18" aria-hidden="true" />{/snippet}
				</Button>
				<Button text="Login" href="/login" class="min-h-12 rounded-lg text-[11px] leading-4 font-semibold tracking-wider text-[#bdc8d1] uppercase shadow-none border-[#3e484f] bg-transparent hover:bg-[#252b2e] hover:text-[#dee3e8]">
					{#snippet icon()}
						<ArrowLeft size="18" aria-hidden="true" />
					{/snippet}
				</Button>
			</div>
			<div class="mt-7 flex items-center justify-center gap-1.5 max-[760px]:mt-5">
				<p class="m-0 text-[13px] leading-4.5 text-[#87929a]">Powered by <span class="font-semibold text-[#dee3e8]">Arkive Core</span></p>
				<BadgeCheck class="size-3.5 stroke-2 text-[#87929a]" aria-hidden="true" />
			</div>
		</section>
	</main>
</div>
