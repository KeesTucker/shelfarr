<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/auth.svelte';
	import { theme } from '$lib/theme.svelte';
	import Nav from '$lib/components/nav.svelte';

	let { children } = $props();
	let ready = $state(false);

	onMount(async () => {
		theme.init();
		if (!authStore.user) {
			await authStore.restore();
		}
		if (!authStore.user) {
			goto('/login');
		} else {
			ready = true;
		}
	});
</script>

{#if ready}
	<div class="min-h-screen bg-sepia-50 text-sepia-900 dark:bg-sepia-950 dark:text-sepia-100">
		<Nav />
		{@render children()}
	</div>
{/if}
