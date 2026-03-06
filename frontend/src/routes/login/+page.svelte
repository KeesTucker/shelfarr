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
			<img src="/logo.svg" alt="" class="mx-auto w-12 h-12 mb-3 dark:invert" aria-hidden="true" />
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
