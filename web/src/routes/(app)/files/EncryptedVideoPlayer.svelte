<script lang="ts">
	import { onMount } from 'svelte';
	import '@videojs/html/video/player';
	import '@videojs/html/video/compat-skin';
	import { openEncryptedStream, type EncryptedStream } from '#lib/streaming/source.ts';
	import { canStreamVideo } from '#lib/utils/video.ts';
	import { DecryptedFileReader } from '#lib/vault/read-file.ts';

	let { fileId, mimeType }: { fileId: string; mimeType: string } = $props();
	let media: HTMLVideoElement;
	let status = $state('Preparing encrypted stream…');
	let errorMessage = $state('');

	onMount(() => {
		const controller = new AbortController();
		const reader = new DecryptedFileReader(fileId, controller.signal);
		let file: Awaited<ReturnType<typeof reader.load>> | undefined;
		let stream: EncryptedStream | undefined;
		let disposed = false;
		let recovering = false;
		let recoveryAttempts = 0;

		function attach(nextStream: EncryptedStream, resumeTime = 0): void {
			stream = nextStream;
			if (resumeTime > 0) {
				const restoreTime = () => {
					media.removeEventListener('loadedmetadata', restoreTime);
					try { media.currentTime = resumeTime; } catch {}
				};
				media.addEventListener('loadedmetadata', restoreTime);
			}
			media.src = nextStream.url;
			media.load();
		}

		async function recoverPlayback(): Promise<void> {
			if (disposed || recovering) return;
			if (recoveryAttempts >= 2) {
				errorMessage = 'Video playback failed. Check your connection and try again.';
				return;
			}
			recovering = true;
			recoveryAttempts += 1;
			status = 'Reconnecting…';
			const resumeTime = media.currentTime;
			const wasPlaying = !media.paused;
			const previousStream = stream;

			try {
				if (!file) throw new Error('The encrypted file is not ready.');
				const nextStream = await openEncryptedStream({ reader, record: file.record, metadata: file.metadata });
				if (disposed) {
					nextStream.close();
					return;
				}
				previousStream?.close();
				errorMessage = '';
				attach(nextStream, resumeTime);
				if (wasPlaying) void media.play().catch(() => {});
			} catch (error) {
				status = '';
				errorMessage = error instanceof Error ? error.message : 'Could not reconnect the encrypted stream.';
			} finally {
				recovering = false;
				if (!errorMessage) status = '';
			}
		}

		const onMediaError = () => { void recoverPlayback(); };
		media.addEventListener('error', onMediaError);

		void (async () => {
			try {
				file = await reader.load();
				const actualMimeType = typeof file.metadata.mime === 'string' ? file.metadata.mime : mimeType;
				if (!canStreamVideo(actualMimeType)) throw new Error('This browser cannot stream this video format.');
				if (disposed) return;
				const firstStream = await openEncryptedStream({ reader, record: file.record, metadata: file.metadata });
				if (disposed) {
					firstStream.close();
					return;
				}
				status = '';
				attach(firstStream);
			} catch (error) {
				if (disposed) return;
				errorMessage = error instanceof Error ? error.message : 'Could not open this video.';
				status = '';
			}
		})();

		return () => {
			disposed = true;
			controller.abort();
			media.removeEventListener('error', onMediaError);
			stream?.close();
			media.pause();
			media.removeAttribute('src');
			media.load();
			void reader.dispose();
		};
	});
</script>

<div class="grid gap-2">
	<div class="overflow-hidden rounded-md border border-(--border) bg-black">
		<video-player class="block w-full">
			<video-compat-skin class="block aspect-video w-full" style="--media-accent-color: var(--accent); --media-accent-text-color: var(--accent-ink); --media-border-color: transparent; --media-border-radius: 0.375rem; --media-font-family: var(--font-sans)">
				<video bind:this={media} class="size-full" playsinline preload="metadata">
					<p>Your browser does not support video playback.</p>
				</video>
			</video-compat-skin>
		</video-player>
	</div>
	{#if status}<p class="m-0 text-sm text-(--muted)" role="status">{status}</p>{/if}
	{#if errorMessage}<p class="m-0 text-sm text-red-400" role="alert">{errorMessage}</p>{/if}
</div>
