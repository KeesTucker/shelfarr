<script lang="ts">
	import { onMount } from 'svelte';
	import { theme } from '$lib/theme.svelte';

	onMount(() => theme.init());
</script>

<div class="min-h-screen bg-sepia-50 px-4 py-16 dark:bg-sepia-950">
	<div class="mx-auto max-w-lg">
		<div class="mb-8">
			<a href="/" class="text-sm text-sepia-500 hover:text-sepia-800 transition-colors dark:text-sepia-500 dark:hover:text-sepia-200">← Back</a>
		</div>

		<h1 class="text-3xl font-bold text-sepia-800 mb-8 dark:text-sepia-100 font-playfair">
			Attributions
		</h1>

		<div class="rounded-xl border border-sepia-400 bg-sepia-100 p-6 dark:border-sepia-700 dark:bg-sepia-900">
			<h2 class="text-xs font-medium text-sepia-500 uppercase tracking-widest mb-4">Icons</h2>
			<p class="text-sm text-sepia-700 leading-relaxed dark:text-sepia-300">
				books by Contributor Icons from
				<a
					href="https://thenounproject.com/browse/icons/term/books/"
					target="_blank"
					rel="noopener noreferrer"
					class="text-sepia-800 underline underline-offset-2 hover:text-sepia-600 dark:text-sepia-200 dark:hover:text-sepia-400"
				>Noun Project</a>
				(CC BY 3.0)
			</p>
		</div>
	</div>
</div>
