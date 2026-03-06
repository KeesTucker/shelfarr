<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { Loader2, FolderInput, Trash2 } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { authStore } from '$lib/auth.svelte';
	import { formatDate, fileTypeClass } from '$lib/utils';
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
		mediaType: string;
		torrentName?: string;
		error?: string;
		finalPath?: string;
		createdAt: string;
		updatedAt: string;
		username?: string;
	}

	interface WatchDirEntry {
		name: string;
	}

	let requests = $state<Request[]>([]);
	let loading = $state(true);
	let error = $state('');
	let intervalId: ReturnType<typeof setInterval> | null = null;

	let importOpen = $state(false);
	let importStep = $state<'pick' | 'fill'>('pick');
	let watchEntries = $state<WatchDirEntry[]>([]);
	let watchLoading = $state(false);
	let watchError = $state('');
	let selectedEntry = $state('');
	let importTitle = $state('');
	let importAuthor = $state('');
	let importMediaType = $state<'audiobook' | 'ebook'>('audiobook');
	let importing = $state(false);

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

	onMount(() => { refresh(); intervalId = setInterval(refresh, 15_000); });
	onDestroy(() => { if (intervalId !== null) clearInterval(intervalId); });

	async function openImport() {
		importStep = 'pick'; selectedEntry = ''; importTitle = ''; importAuthor = '';
		importMediaType = 'audiobook'; watchEntries = []; watchError = '';
		watchLoading = true; importOpen = true;
		try {
			watchEntries = await api.get<WatchDirEntry[]>('/api/watchdir');
		} catch (e) {
			watchError = e instanceof Error ? e.message : 'Failed to load watch directory';
		} finally {
			watchLoading = false;
		}
	}

	function pickEntry(name: string) { selectedEntry = name; importTitle = name; importAuthor = ''; importStep = 'fill'; }

	async function submitImport() {
		if (!importTitle.trim() || !importAuthor.trim()) return;
		importing = true;
		try {
			await api.post('/api/import', { torrentName: selectedEntry, title: importTitle.trim(), author: importAuthor.trim(), mediaType: importMediaType });
			importOpen = false;
			showToast(`"${importTitle.trim()}" queued for import`, 'success');
			refresh();
		} catch (e) {
			showToast(e instanceof Error ? e.message : 'Import failed', 'error');
		} finally {
			importing = false;
		}
	}

	let deleting = $state<Set<string>>(new Set());

	async function deleteRequest(req: Request) {
		deleting = new Set([...deleting, req.id]);
		try {
			await api.delete(`/api/requests/${req.id}`);
			requests = requests.filter((r) => r.id !== req.id);
		} catch (e) {
			showToast(e instanceof Error ? e.message : 'Failed to delete request', 'error');
		} finally {
			const next = new Set(deleting); next.delete(req.id); deleting = next;
		}
	}

	function showToast(message: string, type: 'success' | 'error') {
		if (toastTimeout !== null) clearTimeout(toastTimeout);
		toast = { message, type };
		toastTimeout = setTimeout(() => { toast = null; }, 4000);
	}
</script>

<main class="mx-auto max-w-5xl px-4 py-8">
	<div class="flex items-center justify-between mb-6">
		<h1 class="text-2xl font-bold text-sepia-800 dark:text-sepia-100 font-playfair">
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
		<div class="flex items-center justify-center gap-2 py-16 text-sm text-sepia-500">
			<Loader2 class="w-4 h-4 animate-spin" />
			Loading…
		</div>
	{:else if error}
		<div class="rounded-lg border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-400">
			{error}
		</div>
	{:else if requests.length === 0}
		<div class="py-16 text-center text-sm text-sepia-500">
			No requests yet. Head to Search to request an audiobook or ebook.
		</div>
	{:else}
		<p class="text-xs text-sepia-500 mb-3">
			{requests.length} request{requests.length !== 1 ? 's' : ''}
		</p>
		<div class="rounded-xl border border-sepia-400 overflow-hidden dark:border-sepia-700">
			<div class="overflow-x-auto">
				<table class="w-full min-w-[360px] text-sm">
					<thead class="bg-sepia-200 border-b border-sepia-400 dark:bg-sepia-900 dark:border-sepia-700">
						<tr>
							<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide dark:text-sepia-400">Status</th>
							<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide dark:text-sepia-400">Title / Author</th>
							{#if authStore.isAdmin}
								<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide hidden sm:table-cell dark:text-sepia-400">User</th>
							{/if}
							<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide hidden sm:table-cell dark:text-sepia-400">Submitted</th>
							<th class="px-2 py-3"></th>
						</tr>
					</thead>
					<tbody class="divide-y divide-sepia-300 dark:divide-sepia-800">
						{#each requests as req (req.id)}
							<tr class="bg-sepia-50 hover:bg-sepia-100 transition-colors dark:bg-sepia-950 dark:hover:bg-sepia-900">
								<td class="px-4 py-3"><Badge status={req.status} /></td>
								<td class="px-4 py-3">
									<div class="flex items-center gap-2">
										<span class="font-medium text-sepia-900 leading-snug dark:text-sepia-100">{req.title}</span>
										{#if req.mediaType === 'ebook'}
											<span class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide {fileTypeClass('ebook')}">ebook</span>
										{:else}
											<span class="inline-block rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide {fileTypeClass('audio')}">audiobook</span>
										{/if}
									</div>
									<div class="text-xs text-sepia-600 mt-0.5 dark:text-sepia-400">{req.author}</div>
									{#if req.status === 'failed' && req.error}
										<div class="text-xs text-red-700 mt-1 dark:text-red-400">{req.error}</div>
									{/if}
									{#if req.status === 'done' && req.finalPath}
										<div class="text-xs text-sepia-500 mt-1 font-mono truncate max-w-xs">{req.finalPath}</div>
									{/if}
								</td>
								{#if authStore.isAdmin}
									<td class="px-4 py-3 text-xs text-sepia-600 hidden sm:table-cell dark:text-sepia-400">{req.username ?? '—'}</td>
								{/if}
								<td class="px-4 py-3 text-xs text-sepia-600 hidden sm:table-cell tabular-nums dark:text-sepia-400">{formatDate(req.createdAt)}</td>
								<td class="px-2 py-3">
									<button
										class="p-1.5 rounded text-sepia-400 hover:text-red-700 hover:bg-sepia-200 transition-colors disabled:opacity-40 dark:text-sepia-600 dark:hover:text-red-400 dark:hover:bg-sepia-800"
										disabled={deleting.has(req.id)}
										onclick={() => deleteRequest(req)}
										title="Remove request"
									>
										<Trash2 class="w-3.5 h-3.5" />
									</button>
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}
</main>

<Dialog.Root bind:open={importOpen}>
	<Dialog.Content class="max-w-lg">
		{#if importStep === 'pick'}
			<Dialog.Title>Import from watch directory</Dialog.Title>
			<Dialog.Description>Select a file or folder to import into the library.</Dialog.Description>

			{#if watchLoading}
				<div class="flex items-center justify-center gap-2 py-8 text-sm text-sepia-500">
					<Loader2 class="w-4 h-4 animate-spin" />
					Loading…
				</div>
			{:else if watchError}
				<div class="rounded-lg border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-700 my-4 dark:border-red-900 dark:bg-red-950/40 dark:text-red-400">
					{watchError}
				</div>
			{:else if watchEntries.length === 0}
				<div class="py-8 text-center text-sm text-sepia-500">No untracked files found in the watch directory.</div>
			{:else}
				<ul class="mt-4 mb-6 divide-y divide-sepia-300 rounded-lg border border-sepia-400 overflow-hidden max-h-64 overflow-y-auto dark:divide-sepia-700 dark:border-sepia-700">
					{#each watchEntries as entry (entry.name)}
						<li>
							<button
								class="w-full px-4 py-3 text-left text-sm text-sepia-800 hover:bg-sepia-200 transition-colors font-mono truncate dark:text-sepia-200 dark:hover:bg-sepia-800"
								onclick={() => pickEntry(entry.name)}
							>{entry.name}</button>
						</li>
					{/each}
				</ul>
			{/if}

			<div class="flex justify-end">
				<Dialog.Close><Button variant="outline">Cancel</Button></Dialog.Close>
			</div>
		{:else}
			<Dialog.Title>Import: set metadata</Dialog.Title>
			<Dialog.Description>Confirm the title and author. Metadata will be fetched automatically.</Dialog.Description>

			<div class="rounded-lg bg-sepia-200 px-4 py-2.5 mb-5 mt-4 dark:bg-sepia-800">
				<span class="block text-[10px] uppercase tracking-widest text-sepia-500 mb-0.5">File</span>
				<span class="block text-sm text-sepia-700 font-mono truncate dark:text-sepia-300">{selectedEntry}</span>
			</div>

			<div class="space-y-4 mb-6">
				<div class="space-y-1.5">
					<Label>Type</Label>
					<div class="flex rounded-lg border border-sepia-400 overflow-hidden text-sm w-fit dark:border-sepia-700">
						<button
							class="px-4 py-1.5 transition-colors {importMediaType === 'audiobook' ? 'bg-sepia-300 text-sepia-900 dark:bg-sepia-700 dark:text-sepia-100' : 'bg-sepia-100 text-sepia-500 hover:text-sepia-800 dark:bg-sepia-900 dark:text-sepia-400 dark:hover:text-sepia-200'}"
							onclick={() => (importMediaType = 'audiobook')}
						>Audiobook</button>
						<button
							class="px-4 py-1.5 transition-colors {importMediaType === 'ebook' ? 'bg-sepia-300 text-sepia-900 dark:bg-sepia-700 dark:text-sepia-100' : 'bg-sepia-100 text-sepia-500 hover:text-sepia-800 dark:bg-sepia-900 dark:text-sepia-400 dark:hover:text-sepia-200'}"
							onclick={() => (importMediaType = 'ebook')}
						>Ebook</button>
					</div>
				</div>
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
				<Button variant="outline" onclick={() => (importStep = 'pick')} disabled={importing}>Back</Button>
				<div class="flex gap-3">
					<Dialog.Close><Button variant="outline" disabled={importing}>Cancel</Button></Dialog.Close>
					<Button onclick={submitImport} disabled={importing || !importTitle.trim() || !importAuthor.trim()}>
						{importing ? 'Importing…' : 'Import'}
					</Button>
				</div>
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>

{#if toast}
	<div
		class="fixed bottom-6 right-6 z-[60] max-w-sm rounded-xl border px-4 py-3 text-sm shadow-xl transition-all {toast.type === 'success'
			? 'border-green-400 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-950 dark:text-green-300'
			: 'border-red-400 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-950 dark:text-red-300'}"
	>
		{toast.message}
	</div>
{/if}
