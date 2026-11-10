<script lang="ts">
	let {
		width,
		minWidth = 160,
		maxWidth = 320,
		visible = true,
		onResize,
		onResizeCommit,
	}: {
		width: number;
		minWidth?: number;
		maxWidth?: number;
		visible?: boolean;
		onResize: (width: number) => void;
		onResizeCommit: (width: number) => void;
	} = $props();
	let resizedWidth = 0;

	function resize(event: PointerEvent): void {
		const target = event.currentTarget;
		const panel = target instanceof HTMLElement ? target.parentElement : null;

		if (!panel) return;

		resizedWidth = Math.min(
			maxWidth,
			Math.max(minWidth, event.clientX - panel.getBoundingClientRect().left),
		);
		onResize(resizedWidth);
	}

	function startResize(event: PointerEvent): void {
		const handle = event.currentTarget;

		if (event.button !== 0 || !(handle instanceof HTMLElement)) return;

		handle.setPointerCapture(event.pointerId);
		event.preventDefault();
		resize(event);
	}

	function moveResize(event: PointerEvent): void {
		const handle = event.currentTarget;

		if (handle instanceof HTMLElement && handle.hasPointerCapture(event.pointerId)) resize(event);
	}

	function finishResize(event: PointerEvent): void {
		const handle = event.currentTarget;

		if (handle instanceof HTMLElement && handle.hasPointerCapture(event.pointerId)) {
			handle.releasePointerCapture(event.pointerId);
			onResizeCommit(resizedWidth);
		}
	}

	function resizeByKey(event: KeyboardEvent): void {
		let nextWidth = width;

		switch (event.key) {
			case 'ArrowLeft':
				nextWidth -= 16;
				break;
			case 'ArrowRight':
				nextWidth += 16;
				break;
			case 'Home':
				nextWidth = minWidth;
				break;
			case 'End':
				nextWidth = maxWidth;
				break;
			default:
				return;
		}

		event.preventDefault();
		resizedWidth = Math.min(maxWidth, Math.max(minWidth, nextWidth));
		onResize(resizedWidth);
		onResizeCommit(resizedWidth);
	}
</script>

<!-- svelte-ignore a11y_no_noninteractive_tabindex (The adjustable separator is keyboard focusable.) -->
<!-- svelte-ignore a11y_no_noninteractive_element_interactions (Pointer and keyboard events resize the separator.) -->
<div
	class={`absolute inset-y-0 right-0 z-10 w-1 cursor-col-resize touch-none bg-transparent transition-colors hover:bg-(--accent) focus-visible:bg-(--accent) focus-visible:outline-2 focus-visible:outline-offset-[-2px] focus-visible:outline-(--accent) ${visible ? 'hidden min-[701px]:block' : 'hidden'}`}
	role="separator"
	aria-label="Resize sidebar"
	aria-orientation="vertical"
	aria-valuemin={minWidth}
	aria-valuemax={maxWidth}
	aria-valuenow={width}
	tabindex="0"
	onpointerdown={startResize}
	onpointermove={moveResize}
	onpointerup={finishResize}
	onpointercancel={finishResize}
	onkeydown={resizeByKey}
></div>
