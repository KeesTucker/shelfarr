<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/auth.svelte';
	import Nav from '$lib/components/Nav.svelte';

	let { children } = $props();

	onMount(() => {
		if (!authStore.user) {
			goto('/login');
		}
	});
</script>

{#if authStore.user}
	<div class="min-h-screen bg-zinc-950 text-zinc-50">
		<Nav />
		{@render children()}
	</div>
{/if}
will