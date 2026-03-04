<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/auth.svelte';
	import Nav from '$lib/components/nav.svelte';

	let { children } = $props();
	let ready = $state(false);

	onMount(async () => {
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
	<div class="min-h-screen bg-zinc-950 text-zinc-50">
		<Nav />
		{@render children()}
	</div>
{/if}
