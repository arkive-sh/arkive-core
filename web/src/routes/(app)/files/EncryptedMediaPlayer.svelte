<script lang="ts">
	import { onMount } from 'svelte';
	import '@videojs/html/video/player';
	import '@videojs/html/video/compat-skin';
	import '@videojs/html/audio/player';
	import '@videojs/html/audio/compat-skin';
	import { openEncryptedStream } from '#lib/streaming/source.ts';
	import type { EncryptedStream } from '#lib/streaming/types.ts';
	import { canStreamAudio, canStreamVideo, isAudioMimeType } from '#lib/utils/media.ts';
	import { DecryptedFileReader } from '#lib/vault/read-file.ts';

	let { fileId, fileName, mimeType }: { fileId: string; fileName: string; mimeType: string } =
		$props();
	const isAudio = $derived(isAudioMimeType(mimeType));
	let media = $state<HTMLMediaElement | null>(null);
	let status = $state('Preparing encrypted media stream…');
	let errorMessage = $state('');

	onMount(() => {
		const element = media;

		if (!element) return;
		const player = element as HTMLMediaElement;
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
					player.removeEventListener('loadedmetadata', restoreTime);
					try {
						player.currentTime = resumeTime;
					} catch {}
				};
				player.addEventListener('loadedmetadata', restoreTime);
			}
			player.src = nextStream.url;
			player.load();
		}

		async function recoverPlayback(): Promise<void> {
			if (disposed || recovering) return;
			if (recoveryAttempts >= 2) {
				errorMessage = `${isAudio ? 'Audio' : 'Video'} playback failed. Check your connection and try again.`;

				return;
			}
			recovering = true;
			recoveryAttempts += 1;
			status = 'Reconnecting…';
			const resumeTime = player.currentTime;
			const wasPlaying = !player.paused;
			const previousStream = stream;

			try {
				if (!file) throw new Error('The encrypted file is not ready.');
				const nextStream = await openEncryptedStream({
					reader,
					record: file.record,
					metadata: file.metadata,
				});

				if (disposed) {
					nextStream.close();

					return;
				}
				previousStream?.close();
				errorMessage = '';
				attach(nextStream, resumeTime);
				if (wasPlaying) void player.play().catch(() => {});
			} catch (error) {
				status = '';
				errorMessage =
					error instanceof Error ? error.message : 'Could not reconnect the encrypted stream.';
			} finally {
				recovering = false;
				if (!errorMessage) status = '';
			}
		}

		const onMediaError = () => {
			void recoverPlayback();
		};
		const onPlaybackResumed = () => {
			recoveryAttempts = 0;
		};
		player.addEventListener('error', onMediaError);
		player.addEventListener('playing', onPlaybackResumed);

		void (async () => {
			try {
				file = await reader.load();
				const actualMimeType =
					typeof file.metadata.mime === 'string' ? file.metadata.mime : mimeType;
				const canPlay = isAudio ? canStreamAudio(actualMimeType) : canStreamVideo(actualMimeType);

				if (!canPlay)
					throw new Error(
						`This browser cannot play this ${isAudio ? 'audio' : 'video'} format natively.`,
					);
				if (disposed) return;
				const firstStream = await openEncryptedStream({
					reader,
					record: file.record,
					metadata: file.metadata,
				});

				if (disposed) {
					firstStream.close();

					return;
				}
				status = '';
				attach(firstStream);
			} catch (error) {
				if (disposed) return;
				errorMessage = error instanceof Error ? error.message : 'Could not open this media file.';
				status = '';
			}
		})();

		return () => {
			disposed = true;
			controller.abort();
			player.removeEventListener('error', onMediaError);
			player.removeEventListener('playing', onPlaybackResumed);
			stream?.close();
			player.pause();
			player.removeAttribute('src');
			player.load();
			void reader.dispose();
		};
	});
</script>

<div class="grid gap-2">
	{#if isAudio}
		<div class="overflow-hidden rounded-md border border-(--border) bg-(--surface)">
			<audio-player class="block w-full" content-title={fileName}>
				<audio-compat-skin
					class="block w-full"
					style="--media-accent-color: var(--accent); --media-accent-text-color: var(--accent-ink); --media-border-color: transparent; --media-border-radius: 0.375rem; --media-font-family: var(--font-sans)"
				>
					<audio bind:this={media} class="w-full" preload="metadata">
						<p>Your browser does not support audio playback.</p>
					</audio>
				</audio-compat-skin>
			</audio-player>
		</div>
	{:else}
		<div class="overflow-hidden rounded-md border border-(--border) bg-black">
			<video-player class="block w-full">
				<video-compat-skin
					class="block aspect-video w-full"
					style="--media-accent-color: var(--accent); --media-accent-text-color: var(--accent-ink); --media-border-color: transparent; --media-border-radius: 0.375rem; --media-font-family: var(--font-sans)"
				>
					<video bind:this={media} class="size-full" playsinline preload="metadata">
						<p>Your browser does not support video playback.</p>
					</video>
				</video-compat-skin>
			</video-player>
		</div>
	{/if}
	{#if status}<p class="m-0 text-sm text-(--muted)" role="status">{status}</p>{/if}
	{#if errorMessage}<p class="m-0 text-sm text-red-400" role="alert">{errorMessage}</p>{/if}
</div>
