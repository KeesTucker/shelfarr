import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
export default {
	preprocess: vitePreprocess(),
	kit: {
		adapter: adapter({
			pages: 'dist',
			assets: 'dist',
			// SPA fallback: Go's spaHandler already handles this in production,
			// but this makes `vite preview` work correctly too.
			fallback: 'index.html',
		}),
	},
};
