<script lang="ts">
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { Sun, Moon, Info } from 'lucide-svelte';
	import { authStore } from '$lib/auth.svelte';
	import { theme } from '$lib/theme.svelte';
	import { Button } from '$lib/components/ui/button';
</script>

<nav class="sticky top-0 z-10 border-b border-sepia-400 bg-sepia-100 dark:border-sepia-700 dark:bg-sepia-900">
	<div class="mx-auto max-w-5xl px-4 py-2 flex flex-wrap items-center justify-between gap-y-2">
		<!-- Brand -->
		<div class="flex items-center gap-2 select-none">
			<svg xmlns="http://www.w3.org/2000/svg" viewBox="-5.0 -10.0 110.0 110.0" class="w-7 h-7 fill-sepia-800 dark:fill-sepia-100" aria-hidden="true">
				<path d="m90.668 58.766c-0.78125-0.29688-1.3438-3.1094-1.5977-4.0703h-51.316c-4.1055-0.039062-4.1328-6.1992 0-6.2617h51.316c0.27734-0.99609 0.80469-3.7812 1.5977-4.0703 4.0898-0.066406 4.1289-6.1836 0-6.2617h-12.93c0.007813-1.8203-1.5742-3.2539-3.3828-3.1328-0.625-1.3164-1.0977-2.6602-1.3477-4.0703-2.0312 0.007812-47.676-0.007813-51.348 0-4.082-0.058594-4.1328-6.1875 0-6.2617 3.8516-0.003907 49.156 0.003906 51.348 0 0.26563-1.0703 0.82422-3.9258 1.5977-4.1016 4.0977-0.039062 4.125-6.1914 0-6.2617h-55.012c-17.758 0.55859-17.82 26.383 0 26.957h7.4531c-5.043 3.918-6.4258 12.16-2.7539 17.535h-4.6953c-17.785 0.55469-17.793 26.398 0 26.957h55.012c4.1133-0.089844 4.1211-6.1953 0-6.2617-0.76172-0.14453-1.3828-3.1836-1.5977-4.0703-2.043-0.011719-47.668 0.007813-51.348 0-4.0977-0.082031-4.1289-6.1758 0-6.2617 3.8672 0.003906 49.141-0.003906 51.348 0 0.23047-0.89844 0.76562-3.8945 1.5977-4.1016h16.062c4.0938-0.078125 4.1289-6.1797 0-6.2617z"/>
			</svg>
			<span class="font-bold text-sepia-800 dark:text-sepia-100" style="font-family: 'Playfair Display', serif;">Shelfarr</span>
			<a href="/attributions" title="Attributions" class="text-sepia-600 hover:text-sepia-900 transition-colors dark:text-sepia-400 dark:hover:text-sepia-100">
				<Info class="w-3 h-3" />
			</a>
		</div>

		<!-- Right side -->
		<div class="flex items-center flex-wrap gap-2">
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
			{#if authStore.isAdmin}
				<Button
					variant={$page.url.pathname === '/library' ? 'default' : 'ghost'}
					size="sm"
					onclick={() => goto('/library')}
				>Library</Button>
			{/if}

			{#if authStore.user}
				<span class="text-sm text-sepia-500 hidden sm:inline">{authStore.user.username}</span>
			{/if}

			<Button variant="ghost" size="icon" onclick={() => theme.toggle()} title="Toggle theme">
				{#if theme.dark}
					<Sun class="w-4 h-4" />
				{:else}
					<Moon class="w-4 h-4" />
				{/if}
			</Button>

			<Button variant="outline" size="sm" onclick={() => authStore.logout()}>Sign out</Button>
		</div>
	</div>
</nav>
