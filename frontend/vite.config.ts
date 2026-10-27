import { defineConfig } from 'vite';
import { sveltekit } from '@sveltejs/kit/vite';
import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
	plugins: [
		sveltekit({
			preprocess: vitePreprocess(),
			// Kit 3 dropped $lib for #lib (subpath imports, which want file
			// extensions); this keeps the existing imports as they are.
			alias: { $lib: 'src/lib' },
			adapter: adapter({
				pages: 'dist',
				assets: 'dist',
				// SPA fallback: Go's spaHandler already handles this in production,
				// but this makes `vite preview` work correctly too.
				fallback: 'index.html',
			}),
		}),
		tailwindcss(),
	],
	server: {
		proxy: {
			'/api': 'http://localhost:8008',
		},
	},
});
