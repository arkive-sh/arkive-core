import adapter from '@sveltejs/adapter-node';
import { sveltekit } from '@sveltejs/kit/vite';
import tailwindcss from '@tailwindcss/vite';
import { defineConfig } from 'vite';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const repositoryRoot = resolve(fileURLToPath(new URL('..', import.meta.url)));

export default defineConfig(({ command }) => ({
	plugins: [
		tailwindcss(),
		sveltekit({
			adapter: adapter(),
			serviceWorker: { register: false },
			csp: {
				mode: 'auto',
				directives: {
					'default-src': ['self'],
					'base-uri': ['self'],
					'object-src': ['none'],
					'frame-ancestors': ['none'],
					'form-action': ['self'],
					'script-src': ['self', 'wasm-unsafe-eval'],
					'script-src-attr': ['none'],
					'worker-src': command === 'serve' ? ['self', 'blob:'] : ['self'],
					'style-src': ['self', 'unsafe-inline', 'https://fonts.googleapis.com'],
					'font-src': ['self', 'https://fonts.gstatic.com', 'data:'],
					'img-src': ['self', 'data:', 'blob:', 'https:', 'http:'],
					'media-src': ['self', 'blob:', 'https:', 'http:'],
					'connect-src': ['self', 'https:', 'http:', 'ws:', 'wss:'],
				},
			},
		}),
	],
	server: { fs: { allow: [repositoryRoot] } },
}));
