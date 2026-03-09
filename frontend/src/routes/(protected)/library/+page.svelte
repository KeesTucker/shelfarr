<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { Loader2, CheckCircle2, AlertTriangle, Wand2, Trash2, RefreshCw } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { fileTypeClass } from '$lib/utils';
	import { authStore } from '$lib/auth.svelte';
	import { Button } from '$lib/components/ui/button';

	interface FileInfo {
		audio: string[] | null;
		ebook: string[] | null;
		metadata: string[] | null;
		images: string[] | null;
		other: string[] | null;
	}

	interface BookMeta { title: string; author: string; year?: number; }

	interface BookEntry {
		author_folder: string;
		title_folder: string;
		path: string;
		metadata: BookMeta;
		metadata_source: 'opf' | 'abs_json' | 'folder';
		needs_rename: boolean;
		needs_flat: boolean;
		needs_encode: boolean;
		is_multi_part: boolean;
		expected_author: string;
		expected_title: string;
		files: FileInfo;
	}

	interface CleanupResult { cleaned: number; errors: string[] | null; }

	let books = $state<BookEntry[]>([]);
	let loading = $state(true);
	let error = $state('');
	let cleaning = $state<Set<string>>(new Set());
	let submitted = $state<Set<string>>(new Set());
	let cleaningAll = $state(false);
	let pruning = $state(false);
	let toast = $state<{ message: string; type: 'success' | 'error' } | null>(null);
	let toastTimeout: ReturnType<typeof setTimeout> | null = null;

	onMount(async () => {
		if (!authStore.isAdmin) { goto('/'); return; }
		await load();
	});

	async function load() {
		loading = true; error = '';
		try {
			books = await api.get<BookEntry[]>('/api/library');
			const encodeNeeded = new Set(books.filter((b) => b.needs_encode).map((b) => `${b.author_folder}/${b.title_folder}`));
			submitted = new Set([...submitted].filter((k) => encodeNeeded.has(k)));
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
			const res = await api.post<CleanupResult>('/api/library/cleanup', { author: book.author_folder, title: book.title_folder });
			const label = res.cleaned > 0 ? `"${book.title_folder}" cleaned` : book.needs_encode ? `"${book.title_folder}" queued for encoding` : `"${book.title_folder}" cleaned`;
			showToast(res.errors?.length ? `Cleaned with errors: ${res.errors.join('; ')}` : label, res.errors?.length ? 'error' : 'success');
			if (book.needs_encode) {
				submitted = new Set([...submitted, key]);
			}
			await load();
		} catch (e) {
			showToast(e instanceof Error ? e.message : 'Cleanup failed', 'error');
		} finally {
			const next = new Set(cleaning); next.delete(key); cleaning = next;
		}
	}

	async function pruneEmptyDirs() {
		pruning = true;
		try {
			const res = await api.post<{ removed: number }>('/api/library/prune', {});
			showToast(res.removed === 0 ? 'No empty directories found' : `Removed ${res.removed} empty director${res.removed !== 1 ? 'ies' : 'y'}`, 'success');
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
			const booksMap = new Map(books.map((b) => [`${b.author_folder}/${b.title_folder}`, b]));
			const keys = [...booksMap.keys()].filter((k) => {
				const b = booksMap.get(k)!;
				return b.needs_rename || b.needs_flat || b.needs_encode;
			});
			const res = await api.post<CleanupResult>('/api/library/cleanup', {});
			const errorKeys = new Set((res.errors ?? []).map((e: string) => {
				const idx = e.indexOf(': ');
				return idx >= 0 ? e.slice(0, idx) : '\x00';
			}));
			const successKeys = keys.filter((k) => !errorKeys.has(k));
			const encodeQueued = successKeys.filter((k) => booksMap.get(k)?.needs_encode).length;
			const parts: string[] = [];
			if (res.cleaned > 0) parts.push(`Cleaned ${res.cleaned} book${res.cleaned !== 1 ? 's' : ''}`);
			if (encodeQueued > 0) parts.push(`${encodeQueued} queued for encoding`);
			const msg = parts.length > 0 ? parts.join(', ') : 'Nothing to clean';
			const hasErrors = (res.errors?.length ?? 0) > 0;
			showToast(hasErrors ? `${msg} — ${res.errors!.length} error(s): ${res.errors!.join('; ')}` : msg, hasErrors ? 'error' : 'success');
			submitted = new Set([...submitted, ...successKeys]);
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
		toastTimeout = setTimeout(() => { toast = null; }, 5000);
	}

	function sourceLabel(src: string) {
		if (src === 'opf') return 'OPF';
		if (src === 'abs_json') return 'ABS JSON';
		return 'folder';
	}

	const needsCleanupCount = $derived(books.filter((b) => b.needs_rename || b.needs_flat || b.needs_encode).length);
	const unsubmittedActionable = $derived(books.filter((b) => (b.needs_rename || b.needs_flat || b.needs_encode) && !submitted.has(`${b.author_folder}/${b.title_folder}`)).length);
</script>

<main class="mx-auto max-w-6xl px-4 py-8">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-2xl font-bold text-sepia-800 dark:text-sepia-100 font-playfair">Library</h1>
			{#if !loading && !error}
				<p class="text-xs text-sepia-500 mt-1">
					{books.length} book{books.length !== 1 ? 's' : ''}
					{#if needsCleanupCount > 0}
						· <span class="text-amber-700 dark:text-amber-400">{needsCleanupCount} need{needsCleanupCount !== 1 ? '' : 's'} cleanup</span>
					{/if}
				</p>
			{/if}
		</div>
		<div class="flex items-center gap-2">
			<Button variant="outline" onclick={load} disabled={loading}>
				<RefreshCw class="w-4 h-4 {loading ? 'animate-spin' : ''}" />
			</Button>
			<Button variant="outline" onclick={pruneEmptyDirs} disabled={pruning}>
				{#if pruning}
					<Loader2 class="w-4 h-4 mr-2 animate-spin" />Pruning…
				{:else}
					<Trash2 class="w-4 h-4 mr-2" />Prune empty dirs
				{/if}
			</Button>
			{#if unsubmittedActionable > 0}
				<Button onclick={cleanAll} disabled={cleaningAll}>
					{#if cleaningAll}
						<Loader2 class="w-4 h-4 mr-2 animate-spin" />Cleaning…
					{:else}
						<Wand2 class="w-4 h-4 mr-2" />Clean all ({unsubmittedActionable})
					{/if}
				</Button>
			{/if}
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center gap-2 py-16 text-sm text-sepia-500">
			<Loader2 class="w-4 h-4 animate-spin" />
			Scanning library…
		</div>
	{:else if error}
		<div class="rounded-lg border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-400">
			{error}
		</div>
	{:else if books.length === 0}
		<div class="py-16 text-center text-sm text-sepia-500">No books found in the library directory.</div>
	{:else}
		<div class="rounded-xl border border-sepia-400 overflow-hidden dark:border-sepia-700">
			<div class="overflow-x-auto">
				<table class="w-full text-sm">
					<thead class="bg-sepia-200 border-b border-sepia-400 dark:bg-sepia-900 dark:border-sepia-700">
						<tr>
							<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide dark:text-sepia-400">Author</th>
							<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide dark:text-sepia-400">Title</th>
							<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide hidden md:table-cell dark:text-sepia-400">Files</th>
							<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide hidden sm:table-cell dark:text-sepia-400">Source</th>
							<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide dark:text-sepia-400">Status</th>
							<th class="px-2 py-3"></th>
						</tr>
					</thead>
					<tbody class="divide-y divide-sepia-300 dark:divide-sepia-800">
						{#each books as book (`${book.author_folder}/${book.title_folder}`)}
							{@const key = `${book.author_folder}/${book.title_folder}`}
							{@const isCleaning = cleaning.has(key)}
							{@const isSubmitted = submitted.has(key)}
							<tr class="bg-sepia-50 hover:bg-sepia-100 transition-colors dark:bg-sepia-950 dark:hover:bg-sepia-900">
								<td class="px-4 py-3">
									<div class="text-sepia-900 leading-snug dark:text-sepia-100">{book.author_folder}</div>
									{#if book.needs_rename && book.expected_author !== book.author_folder}
										<div class="text-xs text-amber-700 mt-0.5 dark:text-amber-400">→ {book.expected_author}</div>
									{/if}
								</td>
								<td class="px-4 py-3">
									<div class="text-sepia-900 leading-snug dark:text-sepia-100">{book.title_folder}</div>
									{#if book.needs_rename && book.expected_title !== book.title_folder}
										<div class="text-xs text-amber-700 mt-0.5 dark:text-amber-400">→ {book.expected_title}</div>
									{/if}
								</td>
								<td class="px-4 py-3 hidden md:table-cell">
									<div class="flex flex-wrap gap-1">
										{#each book.files.audio ?? [] as ext}
											<span class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium {fileTypeClass('audio')}">{ext}</span>
										{/each}
										{#each book.files.ebook ?? [] as ext}
											<span class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium {fileTypeClass('ebook')}">{ext}</span>
										{/each}
										{#each book.files.metadata ?? [] as ext}
											<span class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium {fileTypeClass('metadata')}">{ext}</span>
										{/each}
										{#each book.files.images ?? [] as ext}
											<span class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium {fileTypeClass('image')}">{ext}</span>
										{/each}
										{#each book.files.other ?? [] as ext}
											<span class="inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-medium {fileTypeClass('other')}">{ext}</span>
										{/each}
									</div>
								</td>
								<td class="px-4 py-3 hidden sm:table-cell">
									<span class="text-xs text-sepia-500">{sourceLabel(book.metadata_source)}</span>
								</td>
								<td class="px-4 py-3">
									{#if book.needs_rename}
										<div class="flex items-center gap-1.5 text-amber-700 dark:text-amber-400">
											<AlertTriangle class="w-3.5 h-3.5 shrink-0" />
											<span class="text-xs">Mismatch</span>
										</div>
									{:else if book.needs_flat}
										<div class="flex items-center gap-1.5 text-blue-700 dark:text-blue-400">
											<AlertTriangle class="w-3.5 h-3.5 shrink-0" />
											<span class="text-xs">Nested</span>
										</div>
									{:else if book.needs_encode}
										<div class="flex items-center gap-1.5 text-purple-700 dark:text-purple-400">
											<AlertTriangle class="w-3.5 h-3.5 shrink-0" />
											<span class="text-xs">Encode</span>
										</div>
									{:else}
										<div class="flex items-center gap-1.5 text-green-700 dark:text-green-400">
											<CheckCircle2 class="w-3.5 h-3.5 shrink-0" />
											<span class="text-xs">OK</span>
										</div>
									{/if}
								</td>
								<td class="px-2 py-3">
									<button
										class="flex items-center gap-1.5 px-2.5 py-1 rounded text-xs text-sepia-500 hover:text-sepia-900 hover:bg-sepia-200 transition-colors disabled:opacity-40 disabled:cursor-not-allowed dark:text-sepia-500 dark:hover:text-sepia-100 dark:hover:bg-sepia-800"
										disabled={(!book.needs_rename && !book.needs_flat && !book.needs_encode) || isCleaning || isSubmitted}
										onclick={() => cleanBook(book)}
										title={isSubmitted ? 'Encoding queued in ABS' : 'Clean up this book'}
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
		class="fixed bottom-6 right-6 z-[60] max-w-sm rounded-xl border px-4 py-3 text-sm shadow-xl transition-all {toast.type === 'success'
			? 'border-green-400 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-950 dark:text-green-300'
			: 'border-red-400 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-950 dark:text-red-300'}"
	>
		{toast.message}
	</div>
{/if}
