<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/auth.svelte';
	import { theme } from '$lib/theme.svelte';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Card, CardHeader, CardContent } from '$lib/components/ui/card';

	let username = $state('');
	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	onMount(() => theme.init());

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		loading = true;
		error = '';
		try {
			await authStore.login(username, password);
			goto('/');
		} catch (err) {
			error = err instanceof Error ? err.message : 'Login failed';
		} finally {
			loading = false;
		}
	}
</script>

<div class="min-h-screen flex items-center justify-center bg-sepia-50 px-4 dark:bg-sepia-950">
	<Card class="w-full max-w-sm p-8">
		<CardHeader>
			<svg xmlns="http://www.w3.org/2000/svg" viewBox="-5.0 -10.0 110.0 135.0" class="mx-auto w-12 h-12 mb-3 fill-sepia-800 dark:fill-sepia-100" aria-hidden="true">
				<path d="m90.668 58.766c-0.78125-0.29688-1.3438-3.1094-1.5977-4.0703h-51.316c-4.1055-0.039062-4.1328-6.1992 0-6.2617h51.316c0.27734-0.99609 0.80469-3.7812 1.5977-4.0703 4.0898-0.066406 4.1289-6.1836 0-6.2617h-12.93c0.007813-1.8203-1.5742-3.2539-3.3828-3.1328-0.625-1.3164-1.0977-2.6602-1.3477-4.0703-2.0312 0.007812-47.676-0.007813-51.348 0-4.082-0.058594-4.1328-6.1875 0-6.2617 3.8516-0.003907 49.156 0.003906 51.348 0 0.26563-1.0703 0.82422-3.9258 1.5977-4.1016 4.0977-0.039062 4.125-6.1914 0-6.2617h-55.012c-17.758 0.55859-17.82 26.383 0 26.957h7.4531c-5.043 3.918-6.4258 12.16-2.7539 17.535h-4.6953c-17.785 0.55469-17.793 26.398 0 26.957h55.012c4.1133-0.089844 4.1211-6.1953 0-6.2617-0.76172-0.14453-1.3828-3.1836-1.5977-4.0703-2.043-0.011719-47.668 0.007813-51.348 0-4.0977-0.082031-4.1289-6.1758 0-6.2617 3.8672 0.003906 49.141-0.003906 51.348 0 0.23047-0.89844 0.76562-3.8945 1.5977-4.1016h16.062c4.0938-0.078125 4.1289-6.1797 0-6.2617z"/>
			</svg>
			<h1 class="text-2xl font-bold text-sepia-800 dark:text-sepia-100" style="font-family: 'Playfair Display', serif;">Shelfarr</h1>
			<p class="mt-1 text-sm text-sepia-600 dark:text-sepia-400">Sign in with your AudioBookShelf account</p>
			<p class="mt-0.5 text-sm text-sepia-400 dark:text-sepia-500">Made with love &amp; too much AI for Amelie</p>
		</CardHeader>

		<CardContent>
			<form onsubmit={handleSubmit} class="space-y-4">
				<div class="space-y-1.5">
					<Label for="username">Username</Label>
					<Input
						id="username"
						name="username"
						type="text"
						placeholder="your username"
						autocomplete="username"
						required
						bind:value={username}
					/>
				</div>

				<div class="space-y-1.5">
					<Label for="password">Password</Label>
					<Input
						id="password"
						name="password"
						type="password"
						placeholder="••••••••"
						autocomplete="current-password"
						required
						bind:value={password}
					/>
				</div>

				{#if error}
					<p class="text-sm text-red-700 dark:text-red-400">{error}</p>
				{/if}

				<Button type="submit" class="mt-2 w-full" disabled={loading}>
					{loading ? 'Signing in…' : 'Sign in'}
				</Button>
			</form>
		</CardContent>
	</Card>
</div>
