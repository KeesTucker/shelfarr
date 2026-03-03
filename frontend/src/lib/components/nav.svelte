<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/auth.svelte';
	import { Button } from '$lib/components/ui/button';
</script>

<nav class="sticky top-0 z-10 border-b border-zinc-800 bg-zinc-900">
	<div class="mx-auto max-w-5xl px-4 h-14 flex items-center justify-between">
		<!-- Brand -->
		<div class="flex items-center gap-2 select-none">
			<span aria-hidden="true" class="text-xl">📚</span>
			<span class="font-bold text-zinc-50">Shelfarr</span>
		</div>

		<!-- Right side -->
		<div class="flex items-center gap-3">
			<Button
				variant={$page.url.pathname === '/' ? 'default' : 'ghost'}
				size="sm"
				onclick={() => goto('/')}
			>Search</Button>
			<Button
				variant={$page.url.pathname === '/requests' ? 'default' : 'ghost'}
				size="sm"
				onclick={() => goto('/requests')}
			>{authStore.isAdmin ? 'All Requests' : 'My Requests'}</Button>

			{#if authStore.user}
				<span class="text-sm text-zinc-500 hidden sm:inline">{authStore.user.username}</span>
			{/if}

			<Button variant="outline" size="sm" onclick={() => authStore.logout()}>Sign out</Button>
		</div>
	</div>
</nav>
