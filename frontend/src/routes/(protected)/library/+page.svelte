<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { Loader2, CheckCircle2, AlertTriangle, Wand2, Trash2 } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { authStore } from '$lib/auth.svelte';
	import { Button } from '$lib/components/ui/button';

	interface FileInfo {
		audio: string[] | null;
		metadata: string[] | null;
		images: string[] | null;
		other: string[] | null;
	}

	interface BookMeta {
		title: string;
		author: string;
		year?: number;
	}

	interface BookEntry {
		author_folder: string;
		title_folder: string;
		path: string;
		metadata: BookMeta;
		metadata_source: 'opf' | 'abs_json' | 'folder';
		needs_rename: boolean;
		expected_author: string;
		expected_title: string;
		files: FileInfo;
	}

	interface CleanupResult {
		cleaned: number;
		errors: string[] | null;
	}

	let books = $state<BookEntry[]>([]);
	let loading = $state(true);
	let error = $state('');

	let cleaning = $state<Set<string>>(new Set());
	let cleaningAll = $state(false);
	let pruning = $state(false);

	let toast = $state<{ message: string; type: 'success' | 'error' } | null>(null);
	let toastTimeout: ReturnType<typeof setTimeout> | null = null;

	onMount(async () => {
		if (!authStore.isAdmin) {
			goto('/');
			return;
		}
		await load();
	});

	async function load() {
		loading = true;
		error = '';
		try {
			books = await api.get<BookEntry[]>('/api/library');
		} catch (e) {
			error = e instanceof Error ? e.message : 'Failed to load library';
		} finally {
			loading = false;
		}
	}

	async function cleanBook(book: BookEntry) {
		const key = `${book.author_folder}/${book.title_folder}`;
		cleaning = new Set([...cleaning, key]);
		try {
			const res = await api.post<CleanupResult>('/api/library/cleanup', {
				author: book.author_folder,
				title: book.title_folder
			});
			showToast(
				res.errors?.length
					? `Cleaned with errors: ${res.errors.join('; ')}`
					: `"${book.title_folder}" cleaned`,
				res.errors?.length ? 'error' : 'success'
			);
			await load();
		} catch (e) {
			showToast(e instanceof Error ? e.message : 'Cleanup failed', 'error');
		} finally {
			const next = new Set(cleaning);
			next.delete(key);
			cleaning = next;
		}
	}

	async function pruneEmptyDirs() {
		pruning = true;
		try {
			const res = await api.post<{ removed: number }>('/api/library/prune', {});
			showToast(
				res.removed === 0
					? 'No empty directories found'
					: `Removed ${res.removed} empty director${res.removed !== 1 ? 'ies' : 'y'}`,
				'success'
			);
			await load();
		} catch (e) {
			showToast(e instanceof Error ? e.message : 'Prune failed', 'error');
		} finally {
			pruning = false;
		}
	}

	async function cleanAll() {
		cleaningAll = true;
		try {
			const res = await api.post<CleanupResult>('/api/library/cleanup', {});
			const msg =
				res.errors?.length
					? `Cleaned ${res.cleaned}, ${res.errors.length} error(s): ${res.errors.join('; ')}`
					: `Cleaned ${res.cleaned} book${res.cleaned !== 1 ? 's' : ''}`;
			showToast(msg, res.errors?.length ? 'error' : 'success');
			await load();
		} catch (e) {
			showToast(e instanceof Error ? e.message : 'Cleanup failed', 'error');
		} finally {
			cleaningAll = false;
		}
	}

	function showToast(message: string, type: 'success' | 'error') {
		if (toastTimeout !== null) clearTimeout(toastTimeout);
		toast = { message, type };
		toastTimeout = setTimeout(() => {
			toast = null;
		}, 5000);
	}

	function sourceLabel(src: string) {
		if (src === 'opf') return 'OPF';
		if (src === 'abs_json') return 'ABS JSON';
		return 'folder';
	}

	const needsRenameCount = $derived(books.filter((b) => b.needs_rename).length);
</script>

<main class="mx-auto max-w-6xl px-4 py-8">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-2xl font-bold text-zinc-100">Library</h1>
			{#if !loading && !error}
				<p class="text-xs text-zinc-500 mt-1">
					{books.length} book{books.length !== 1 ? 's' : ''}
					{#if needsRenameCount > 0}
						· <span class="text-amber-400">{needsRenameCount} need{needsRenameCount !== 1 ? '' : 's'} cleanup</span>
					{/if}
				</p>
			{/if}
		</div>
		<div class="flex items-center gap-2">
			<Button variant="outline" onclick={pruneEmptyDirs} disabled={pruning}>
				{#if pruning}
					<Loader2 class="w-4 h-4 mr-2 animate-spin" />
					Pruning…
				{:else}
					<Trash2 class="w-4 h-4 mr-2" />
					Prune empty dirs
				{/if}
			</Button>
			{#if needsRenameCount > 0}
				<Button onclick={cleanAll} disabled={cleaningAll}>
					{#if cleaningAll}
						<Loader2 class="w-4 h-4 mr-2 animate-spin" />
						Cleaning…
					{:else}
						<Wand2 class="w-4 h-4 mr-2" />
						Clean all ({needsRenameCount})
					{/if}
				</Button>
			{/if}
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center gap-2 py-16 text-sm text-zinc-400">
			<Loader2 class="w-4 h-4 animate-spin" />
			Scanning library…
		</div>
	{:else if error}
		<div class="rounded-lg border border-red-900 bg-red-950/40 px-4 py-3 text-sm text-red-400">
			{error}
		</div>
	{:else if books.length === 0}
		<div class="py-16 text-center text-sm text-zinc-500">
			No books found in the library directory.
		</div>
	{:else}
		<div class="rounded-xl border border-zinc-800 overflow-hidden">
			<div class="overflow-x-auto">
				<table class="w-full text-sm">
					<thead class="bg-zinc-900 border-b border-zinc-800">
						<tr>
							<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide">Author</th>
							<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide">Title</th>
							<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide hidden md:table-cell">Files</th>
							<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide hidden sm:table-cell">Source</th>
							<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide">Status</th>
							<th class="px-2 py-3"></th>
						</tr>
					</thead>
					<tbody class="divide-y divide-zinc-800">
						{#each books as book (`${book.author_folder}/${book.title_folder}`)}
							{@const key = `${book.author_folder}/${book.title_folder}`}
							{@const isCleaning = cleaning.has(key)}
							<tr class="bg-zinc-950 hover:bg-zinc-900 transition-colors">
								<td class="px-4 py-3">
									<div class="text-zinc-100 leading-snug">{book.author_folder}</div>
									{#if book.needs_rename && book.expected_author !== book.author_folder}
										<div class="text-xs text-amber-400 mt-0.5">→ {book.expected_author}</div>
									{/if}
								</td>
								<td class="px-4 py-3">
									<div class="text-zinc-100 leading-snug">{book.title_folder}</div>
									{#if book.needs_rename && book.expected_title !== book.title_folder}
										<div class="text-xs text-amber-400 mt-0.5">→ {book.expected_title}</div>
									{/if}
								</td>
								<td class="px-4 py-3 hidden md:table-cell">
									<div class="flex flex-wrap gap-1">
										{#each book.files.audio ?? [] as ext}
											<span class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium bg-blue-950 text-blue-300 border border-blue-800">{ext}</span>
										{/each}
										{#each book.files.metadata ?? [] as ext}
											<span class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium bg-purple-950 text-purple-300 border border-purple-800">{ext}</span>
										{/each}
										{#each book.files.images ?? [] as ext}
											<span class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium bg-zinc-800 text-zinc-400 border border-zinc-700">{ext}</span>
										{/each}
										{#each book.files.other ?? [] as ext}
											<span class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium bg-zinc-800 text-zinc-500 border border-zinc-700">{ext}</span>
										{/each}
									</div>
								</td>
								<td class="px-4 py-3 hidden sm:table-cell">
									<span class="text-xs text-zinc-500">{sourceLabel(book.metadata_source)}</span>
								</td>
								<td class="px-4 py-3">
									{#if book.needs_rename}
										<div class="flex items-center gap-1.5 text-amber-400">
											<AlertTriangle class="w-3.5 h-3.5 shrink-0" />
											<span class="text-xs">Mismatch</span>
										</div>
									{:else}
										<div class="flex items-center gap-1.5 text-green-500">
											<CheckCircle2 class="w-3.5 h-3.5 shrink-0" />
											<span class="text-xs">OK</span>
										</div>
									{/if}
								</td>
								<td class="px-2 py-3">
									<button
										class="flex items-center gap-1.5 px-2.5 py-1 rounded text-xs text-zinc-400 hover:text-zinc-100 hover:bg-zinc-800 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
										disabled={!book.needs_rename || isCleaning}
										onclick={() => cleanBook(book)}
										title="Clean up this book"
									>
										{#if isCleaning}
											<Loader2 class="w-3 h-3 animate-spin" />
										{:else}
											<Wand2 class="w-3 h-3" />
										{/if}
										Clean
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

{#if toast}
	<div
		class="fixed bottom-6 right-6 z-[60] max-w-sm rounded-xl border px-4 py-3 text-sm shadow-2xl transition-all {toast.type === 'success'
			? 'border-green-800 bg-green-950 text-green-300'
			: 'border-red-800 bg-red-950 text-red-300'}"
	>
		{toast.message}
	</div>
{/if}
