<script lang="ts">
	import { tick, untrack } from 'svelte';
	let {
		index,
		bytes,
		wrapLines,
		onChange,
		onKeydown,
		onPaste
	}: {
		index: number;
		bytes: Uint8Array;
		wrapLines: boolean;
		onChange: (index: number, value: string) => boolean;
		onKeydown: (index: number, event: KeyboardEvent) => void;
		onPaste: (index: number, event: ClipboardEvent) => void;
	} = $props();
	let value = $state(new TextDecoder().decode(untrack(() => bytes)));
	let editor = $state<HTMLTextAreaElement>();

	function resizeEditor(): void {
		if (!editor) return;
		if (wrapLines) {
			editor.style.width = '';
			editor.style.height = '28px';
			editor.style.height = `${Math.max(28, editor.scrollHeight)}px`;
		} else {
			editor.style.height = '28px';
			editor.style.width = '1px';
			editor.style.width = `${Math.max((editor.parentElement?.clientWidth ?? 56) - 56, editor.scrollWidth)}px`;
		}
	}

	$effect(() => {
		const next = new TextDecoder().decode(bytes);
		if (untrack(() => value) !== next) value = next;
		void tick().then(resizeEditor);
	});

	$effect(() => {
		const node = editor;
		if (!node) return;
		if (wrapLines) node.style.width = '';
		let size = `${node.clientWidth}:${node.parentElement?.clientWidth ?? 0}`;
		const observer = new ResizeObserver(() => {
			const nextSize = `${node.clientWidth}:${node.parentElement?.clientWidth ?? 0}`;
			if (nextSize === size) return;
			size = nextSize;
			resizeEditor();
		});
		observer.observe(node);
		if (node.parentElement) observer.observe(node.parentElement);
		resizeEditor();
		return () => observer.disconnect();
	});
</script>

<textarea
	class={`m-0 block min-h-7 resize-none overflow-hidden border-0 bg-transparent px-2 font-mono text-sm leading-7 text-(--text) outline-none focus:bg-(--surface-highest) ${wrapLines ? 'min-w-0 flex-1 whitespace-pre-wrap wrap-break-word' : 'w-max min-w-0 flex-none whitespace-pre'}`}
	rows="1"
	data-line-input={index}
	aria-label={`Line ${index + 1}`}
	autocomplete="off"
	autocapitalize="off"
	spellcheck="false"
	bind:this={editor}
	bind:value
	oninput={(event) => {
		if (!onChange(index, event.currentTarget.value)) {
			value = new TextDecoder().decode(bytes);
			event.currentTarget.value = value;
		}
		resizeEditor();
	}}
	onkeydown={(event) => onKeydown(index, event)}
	onpaste={(event) => onPaste(index, event)}
></textarea>
