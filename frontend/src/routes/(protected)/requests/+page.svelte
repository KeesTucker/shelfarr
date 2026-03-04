<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Loader2, FolderInput } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { authStore } from '$lib/auth.svelte';
	import { formatDate } from '$lib/utils';
	import { Badge } from '$lib/components/ui/badge';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import * as Dialog from '$lib/components/ui/dialog';

	interface Request {
		id: string;
		title: string;
		author: string;
		status: string;
		torrentName?: string;
		error?: string;
		finalPath?: string;
		createdAt: string;
		updatedAt: string;
		username?: string; // admin view only
	}

	interface WatchDirEntry {
		name: string;
	}

	let requests = $state<Request[]>([]);
	let loading = $state(true);
	let error = $state('');
	let intervalId: ReturnType<typeof setInterval> | null = null;

	// Import dialog state
	let importOpen = $state(false);
	let importStep = $state<'pick' | 'fill'>('pick');
	let watchEntries = $state<WatchDirEntry[]>([]);
	let watchLoading = $state(false);
	let watchError = $state('');
	let selectedEntry = $state('');
	let importTitle = $state('');
	let importAuthor = $state('');
	let importing = $state(false);

	// Toast state
	let toast = $state<{ message: string; type: 'success' | 'error' } | null>(null);
	let toastTimeout: ReturnType<typeof setTimeout> | null = null;

	async function refresh() {
		try {
			requests = await api.get<Request[]>('/api/requests');
			if (loading) loading = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load requests';
			loading = false;
		}
	}

	onMount(() => {
		refresh();
		intervalId = setInterval(refresh, 15_000);
	});

	onDestroy(() => {
		if (intervalId !== null) clearInterval(intervalId);
	});

	async function openImport() {
		importStep = 'pick';
		selectedEntry = '';
		importTitle = '';
		importAuthor = '';
		watchEntries = [];
		watchError = '';
		watchLoading = true;
		importOpen = true;
		try {
			watchEntries = await api.get<WatchDirEntry[]>('/api/watchdir');
		} catch (e) {
			watchError = e instanceof Error ? e.message : 'Failed to load watch directory';
		} finally {
			watchLoading = false;
		}
	}

	function pickEntry(name: string) {
		selectedEntry = name;
		importTitle = name;
		importAuthor = '';
		importStep = 'fill';
	}

	async function submitImport() {
		if (!importTitle.trim() || !importAuthor.trim()) return;
		importing = true;
		try {
			await api.post('/api/import', {
				torrentName: selectedEntry,
				title: importTitle.trim(),
				author: importAuthor.trim(),
			});
			importOpen = false;
			showToast(`"${importTitle.trim()}" queued for import`, 'success');
			refresh();
		} catch (e) {
			showToast(e instanceof Error ? e.message : 'Import failed', 'error');
		} finally {
			importing = false;
		}
	}

	function showToast(message: string, type: 'success' | 'error') {
		if (toastTimeout !== null) clearTimeout(toastTimeout);
		toast = { message, type };
		toastTimeout = setTimeout(() => {
			toast = null;
		}, 4000);
	}
</script>

<main class="mx-auto max-w-5xl px-4 py-8">
	<div class="flex items-center justify-between mb-6">
		<h1 class="text-2xl font-bold text-zinc-100">
			{authStore.isAdmin ? 'All Requests' : 'My Requests'}
		</h1>
		{#if authStore.isAdmin}
			<Button variant="outline" onclick={openImport}>
				<FolderInput class="w-4 h-4 mr-2" />
				Import files
			</Button>
		{/if}
	</div>

	{#if loading}
		<div class="flex items-center justify-center gap-2 py-16 text-sm text-zinc-400">
			<Loader2 class="w-4 h-4 animate-spin" />
			Loading…
		</div>
	{:else if error}
		<div class="rounded-lg border border-red-900 bg-red-950/40 px-4 py-3 text-sm text-red-400">
			{error}
		</div>
	{:else if requests.length === 0}
		<div class="py-16 text-center text-sm text-zinc-500">
			No requests yet. Head to Search to request an audiobook.
		</div>
	{:else}
		<p class="text-xs text-zinc-500 mb-3">
			{requests.length} request{requests.length !== 1 ? 's' : ''}
		</p>
		<div class="rounded-xl border border-zinc-800 overflow-hidden">
			<div class="overflow-x-auto">
			<table class="w-full min-w-[360px] text-sm">
				<thead class="bg-zinc-900 border-b border-zinc-800">
					<tr>
						<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide"
							>Status</th
						>
						<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide"
							>Title / Author</th
						>
						{#if authStore.isAdmin}
							<th
								class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide hidden sm:table-cell"
								>User</th
							>
						{/if}
						<th
							class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide hidden sm:table-cell"
							>Submitted</th
						>
					</tr>
				</thead>
				<tbody class="divide-y divide-zinc-800">
					{#each requests as req (req.id)}
						<tr class="bg-zinc-950 hover:bg-zinc-900 transition-colors">
							<td class="px-4 py-3">
								<Badge status={req.status} />
							</td>
							<td class="px-4 py-3">
								<div class="font-medium text-zinc-100 leading-snug">{req.title}</div>
								<div class="text-xs text-zinc-400 mt-0.5">{req.author}</div>
								{#if req.status === 'failed' && req.error}
									<div class="text-xs text-red-400 mt-1">{req.error}</div>
								{/if}
								{#if req.status === 'done' && req.finalPath}
									<div class="text-xs text-zinc-500 mt-1 font-mono truncate max-w-xs">
										{req.finalPath}
									</div>
								{/if}
							</td>
							{#if authStore.isAdmin}
								<td class="px-4 py-3 text-xs text-zinc-400 hidden sm:table-cell">
									{req.username ?? '—'}
								</td>
							{/if}
							<td class="px-4 py-3 text-xs text-zinc-400 hidden sm:table-cell tabular-nums">
								{formatDate(req.createdAt)}
							</td>
						</tr>
					{/each}
				</tbody>
			</table>
			</div>
		</div>
	{/if}
</main>

<!-- Import dialog -->
<Dialog.Root bind:open={importOpen}>
	<Dialog.Content class="max-w-lg">
		{#if importStep === 'pick'}
			<Dialog.Title>Import from watch directory</Dialog.Title>
			<Dialog.Description>
				Select a file or folder to import into the library.
			</Dialog.Description>

			{#if watchLoading}
				<div class="flex items-center justify-center gap-2 py-8 text-sm text-zinc-400">
					<Loader2 class="w-4 h-4 animate-spin" />
					Loading…
				</div>
			{:else if watchError}
				<div class="rounded-lg border border-red-900 bg-red-950/40 px-4 py-3 text-sm text-red-400 my-4">
					{watchError}
				</div>
			{:else if watchEntries.length === 0}
				<div class="py-8 text-center text-sm text-zinc-500">
					No untracked files found in the watch directory.
				</div>
			{:else}
				<ul class="mt-4 mb-6 divide-y divide-zinc-800 rounded-lg border border-zinc-800 overflow-hidden max-h-64 overflow-y-auto">
					{#each watchEntries as entry (entry.name)}
						<li>
							<button
								class="w-full px-4 py-3 text-left text-sm text-zinc-200 hover:bg-zinc-800 transition-colors font-mono truncate"
								onclick={() => pickEntry(entry.name)}
							>
								{entry.name}
							</button>
						</li>
					{/each}
				</ul>
			{/if}

			<div class="flex justify-end">
				<Dialog.Close>
					<Button variant="outline">Cancel</Button>
				</Dialog.Close>
			</div>
		{:else}
			<Dialog.Title>Import: set metadata</Dialog.Title>
			<Dialog.Description>
				Confirm the title and author. Metadata will be fetched automatically.
			</Dialog.Description>

			<div class="rounded-lg bg-zinc-800 px-4 py-2.5 mb-5 mt-4">
				<span class="block text-[10px] uppercase tracking-widest text-zinc-500 mb-0.5">File</span>
				<span class="block text-sm text-zinc-300 font-mono truncate">{selectedEntry}</span>
			</div>

			<div class="space-y-4 mb-6">
				<div class="space-y-1.5">
					<Label for="import-title">Title</Label>
					<Input id="import-title" bind:value={importTitle} placeholder="Book title" />
				</div>
				<div class="space-y-1.5">
					<Label for="import-author">Author</Label>
					<Input id="import-author" bind:value={importAuthor} placeholder="Author name" />
				</div>
			</div>

			<div class="flex gap-3 justify-between">
				<Button variant="outline" onclick={() => (importStep = 'pick')} disabled={importing}>
					Back
				</Button>
				<div class="flex gap-3">
					<Dialog.Close>
						<Button variant="outline" disabled={importing}>Cancel</Button>
					</Dialog.Close>
					<Button
						onclick={submitImport}
						disabled={importing || !importTitle.trim() || !importAuthor.trim()}
					>
						{importing ? 'Importing…' : 'Import'}
					</Button>
				</div>
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>

<!-- Toast notification -->
{#if toast}
	<div
		class="fixed bottom-6 right-6 z-[60] max-w-sm rounded-xl border px-4 py-3 text-sm shadow-2xl transition-all {toast.type ===
		'success'
			? 'border-green-800 bg-green-950 text-green-300'
			: 'border-red-800 bg-red-950 text-red-300'}"
	>
		{toast.message}
	</div>
{/if}
