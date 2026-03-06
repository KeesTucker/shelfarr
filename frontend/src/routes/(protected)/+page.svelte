<script lang="ts">
	import { Search, Loader2, Check } from 'lucide-svelte';
	import { api } from '$lib/api';
	import { formatSize, tagColorFromLabel } from '$lib/utils';
	import { Button } from '$lib/components/ui/button';
	import { Badge } from '$lib/components/ui/badge';
	import * as Dialog from '$lib/components/ui/dialog';

	interface SearchResult {
		id: string;
		title: string;
		author: string;
		narrator?: string;
		tags?: string[];
		size: number;
		seeders: number;
		indexer: string;
		publishDate?: string;
	}

	interface Book {
		title: string;
		author: string;
		year?: number;
	}

	let mediaType = $state<'audiobook' | 'ebook'>('audiobook');
	let query = $state('');
	let results = $state<SearchResult[]>([]);
	let loading = $state(false);
	let error = $state('');
	let searched = $state(false);
	let debounceId: ReturnType<typeof setTimeout> | null = null;

	let selected = $state<SearchResult | null>(null);
	let dialogOpen = $state(false);
	let requesting = $state(false);

	let metaResults = $state<Book[]>([]);
	let metaLoading = $state(false);
	let selectedMeta = $state<Book | null>(null);
	let metaSearchError = $state('');
	let metaExpanded = $state(false);
	let metaConfident = $state(false);
	let metaSkipped = $state(false);

	function metaScore(torrentTitle: string, metaTitle: string): number {
		const norm = (s: string) =>
			s.toLowerCase().replace(/[^a-z0-9\s]/g, '').split(/\s+/).filter((w) => w.length > 1);
		const torrentWords = new Set(norm(torrentTitle));
		const metaWords = norm(metaTitle);
		if (metaWords.length === 0) return 0;
		return metaWords.filter((w) => torrentWords.has(w)).length / metaWords.length;
	}

	let toast = $state<{ message: string; type: 'success' | 'error' } | null>(null);
	let toastTimeout: ReturnType<typeof setTimeout> | null = null;

	function handleInput() {
		if (debounceId !== null) clearTimeout(debounceId);
		debounceId = setTimeout(() => doSearch(query.trim()), 400);
	}

	async function doSearch(q: string) {
		if (q.length < 2) { results = []; searched = false; return; }
		loading = true; error = ''; searched = true;
		try {
			results = await api.get<SearchResult[]>(`/api/search?q=${encodeURIComponent(q)}&type=${mediaType}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Search failed';
			results = [];
		} finally {
			loading = false;
		}
	}

	function switchType(t: 'audiobook' | 'ebook') {
		if (t === mediaType) return;
		if (debounceId !== null) clearTimeout(debounceId);
		mediaType = t; results = []; searched = false;
		if (query.trim().length >= 2) doSearch(query.trim());
	}

	function openConfirm(result: SearchResult) {
		selected = result; metaResults = []; selectedMeta = null;
		metaSearchError = ''; metaExpanded = false; metaConfident = false; metaSkipped = false;
		dialogOpen = true;
		searchMetadata(result.title, result.author);
	}

	async function searchMetadata(title: string, author: string) {
		metaLoading = true;
		try {
			const books = await api.get<Book[]>(
				`/api/metadata/search?title=${encodeURIComponent(title)}&author=${encodeURIComponent(author)}`
			);
			metaResults = books ?? [];
			if (metaResults.length > 0) {
				selectedMeta = metaResults[0];
				const score = metaScore(selected?.title ?? '', metaResults[0].title);
				metaConfident = score >= 0.4;
				metaExpanded = !metaConfident;
			}
		} catch {
			metaSearchError = 'Metadata search failed';
		} finally {
			metaLoading = false;
		}
	}

	function pickMeta(book: Book) { selectedMeta = book; metaSkipped = false; metaExpanded = false; }
	function skipMeta() { selectedMeta = null; metaSkipped = true; metaExpanded = false; }

	async function handleRequest() {
		if (!selected) return;
		requesting = true;
		const title = selected.title;
		try {
			await api.post<{ id: string }>('/api/requests', {
				title: selected.title, author: selected.author, torrentGuid: selected.id, mediaType,
				...(selectedMeta ? { metadata: selectedMeta } : {}),
			});
			dialogOpen = false;
			showToast(`"${title}" added to download queue`, 'success');
		} catch (e) {
			dialogOpen = false;
			showToast(e instanceof Error ? e.message : 'Failed to add request', 'error');
		} finally {
			requesting = false;
		}
	}

	function showToast(message: string, type: 'success' | 'error') {
		if (toastTimeout !== null) clearTimeout(toastTimeout);
		toast = { message, type };
		toastTimeout = setTimeout(() => { toast = null; }, 4000);
	}

	function seedColor(n: number): string {
		return n > 10 ? 'text-green-700 dark:text-green-400' : n > 2 ? 'text-amber-700 dark:text-amber-400' : 'text-red-700 dark:text-red-400';
	}
</script>

<main class="mx-auto max-w-5xl px-4 py-8">
	<div class="flex items-center justify-between mb-6">
		<h1 class="text-2xl font-bold text-sepia-800 dark:text-sepia-100" style="font-family: 'Playfair Display', serif;">
			Find a{mediaType === 'ebook' ? 'n Ebook' : 'n Audiobook'}
		</h1>
		<div class="flex rounded-lg border border-sepia-400 overflow-hidden text-sm dark:border-sepia-700">
			<button
				class="px-4 py-1.5 transition-colors {mediaType === 'audiobook' ? 'bg-sepia-300 text-sepia-900 dark:bg-sepia-700 dark:text-sepia-100' : 'bg-sepia-100 text-sepia-500 hover:text-sepia-800 dark:bg-sepia-900 dark:text-sepia-400 dark:hover:text-sepia-200'}"
				onclick={() => switchType('audiobook')}
			>Audiobook</button>
			<button
				class="px-4 py-1.5 transition-colors {mediaType === 'ebook' ? 'bg-sepia-300 text-sepia-900 dark:bg-sepia-700 dark:text-sepia-100' : 'bg-sepia-100 text-sepia-500 hover:text-sepia-800 dark:bg-sepia-900 dark:text-sepia-400 dark:hover:text-sepia-200'}"
				onclick={() => switchType('ebook')}
			>Ebook</button>
		</div>
	</div>

	<div class="relative mb-6">
		<span class="absolute inset-y-0 left-0 flex items-center pl-3 pointer-events-none">
			<Search class="w-5 h-5 text-sepia-400 dark:text-sepia-500" />
		</span>
		<input
			type="search"
			bind:value={query}
			oninput={handleInput}
			placeholder={mediaType === 'ebook' ? 'Search by title, author…' : 'Search by title, author, narrator…'}
			class="w-full rounded-lg border border-sepia-400 bg-sepia-50 pl-10 pr-4 py-3 text-sm text-sepia-900 placeholder:text-sepia-400 focus:border-sepia-600 focus:outline-none focus:ring-1 focus:ring-sepia-600 transition-colors dark:border-sepia-700 dark:bg-sepia-900 dark:text-sepia-100 dark:placeholder:text-sepia-500 dark:focus:border-sepia-500 dark:focus:ring-sepia-500"
		/>
	</div>

	{#if loading}
		<div class="flex items-center justify-center gap-2 py-16 text-sm text-sepia-500">
			<Loader2 class="w-4 h-4 animate-spin" />
			Searching…
		</div>
	{:else if error}
		<div class="rounded-lg border border-red-300 bg-red-50 px-4 py-3 text-sm text-red-700 dark:border-red-900 dark:bg-red-950/40 dark:text-red-400">
			{error}
		</div>
	{:else if searched && results.length === 0}
		<div class="py-16 text-center text-sm text-sepia-500">
			No results found. Try a different search.
		</div>
	{:else if results.length > 0}
		<p class="text-xs text-sepia-500 mb-3">
			{results.length} result{results.length !== 1 ? 's' : ''}
		</p>
		<div class="rounded-xl border border-sepia-400 overflow-hidden dark:border-sepia-700">
			<div class="overflow-x-auto">
				<table class="w-full min-w-[420px] text-sm">
					<thead class="bg-sepia-200 border-b border-sepia-400 dark:bg-sepia-900 dark:border-sepia-700">
						<tr>
							<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide dark:text-sepia-400">Title / Author</th>
							{#if mediaType === 'audiobook'}
								<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide hidden sm:table-cell dark:text-sepia-400">Narrator</th>
							{/if}
							<th class="px-4 py-3 text-left text-xs font-medium text-sepia-600 uppercase tracking-wide hidden md:table-cell dark:text-sepia-400">Indexer</th>
							<th class="px-4 py-3 text-right text-xs font-medium text-sepia-600 uppercase tracking-wide dark:text-sepia-400">Size</th>
							<th class="px-4 py-3 text-right text-xs font-medium text-sepia-600 uppercase tracking-wide dark:text-sepia-400">Seeds</th>
						</tr>
					</thead>
					<tbody class="divide-y divide-sepia-300 dark:divide-sepia-800">
						{#each results as result (result.id)}
							<tr
								class="bg-sepia-50 hover:bg-sepia-100 cursor-pointer transition-colors dark:bg-sepia-950 dark:hover:bg-sepia-900"
								onclick={() => openConfirm(result)}
								onkeydown={(e) => e.key === 'Enter' && openConfirm(result)}
								role="button"
								tabindex="0"
							>
								<td class="px-4 py-3">
									<div class="font-medium text-sepia-900 leading-snug dark:text-sepia-100">{result.title}</div>
									<div class="text-xs text-sepia-600 mt-0.5 dark:text-sepia-400">{result.author}</div>
									{#if result.tags?.length}
										<div class="flex flex-wrap gap-1 mt-1.5">
											{#each result.tags as tag}
												<span class="inline-block rounded px-1.5 py-0.5 text-[10px] font-mono font-medium {tagColorFromLabel(tag)}">{tag}</span>
											{/each}
										</div>
									{/if}
								</td>
								{#if mediaType === 'audiobook'}
									<td class="px-4 py-3 text-xs text-sepia-600 hidden sm:table-cell dark:text-sepia-400">
										{result.narrator ?? '—'}
									</td>
								{/if}
								<td class="px-4 py-3 hidden md:table-cell">
									<Badge class="bg-sepia-200 text-sepia-700 dark:bg-sepia-800 dark:text-sepia-300">{result.indexer}</Badge>
								</td>
								<td class="px-4 py-3 text-right text-xs text-sepia-600 tabular-nums dark:text-sepia-400">
									{formatSize(result.size)}
								</td>
								<td class="px-4 py-3 text-right text-xs font-medium tabular-nums {seedColor(result.seeders)}">
									{result.seeders}
								</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</div>
		</div>
	{/if}
</main>

{#if selected}
	<Dialog.Root bind:open={dialogOpen}>
		<Dialog.Content>
			<Dialog.Title>Confirm Download</Dialog.Title>
			<Dialog.Description>This will add the torrent to the download queue.</Dialog.Description>

			<div class="rounded-lg bg-sepia-200 p-4 space-y-3 dark:bg-sepia-800">
				<div>
					<span class="block text-[10px] uppercase tracking-widest text-sepia-500 mb-0.5">Title</span>
					<span class="block text-sm text-sepia-800 dark:text-sepia-200">{selected.title}</span>
				</div>
				<div>
					<span class="block text-[10px] uppercase tracking-widest text-sepia-500 mb-0.5">Author</span>
					<span class="block text-sm text-sepia-800 dark:text-sepia-200">{selected.author}</span>
				</div>
				{#if selected.tags?.length}
					<div>
						<span class="block text-[10px] uppercase tracking-widest text-sepia-500 mb-1">Format</span>
						<div class="flex flex-wrap gap-1">
							{#each selected.tags as tag}
								<span class="inline-block rounded px-1.5 py-0.5 text-[10px] font-mono font-medium {tagColorFromLabel(tag)}">{tag}</span>
							{/each}
						</div>
					</div>
				{/if}
				{#if mediaType === 'audiobook' && selected.narrator}
					<div>
						<span class="block text-[10px] uppercase tracking-widest text-sepia-500 mb-0.5">Narrator</span>
						<span class="block text-sm text-sepia-800 dark:text-sepia-200">{selected.narrator}</span>
					</div>
				{/if}
				<div class="flex gap-6">
					<div>
						<span class="block text-[10px] uppercase tracking-widest text-sepia-500 mb-0.5">Size</span>
						<span class="block text-sm text-sepia-800 dark:text-sepia-200">{formatSize(selected.size)}</span>
					</div>
					<div>
						<span class="block text-[10px] uppercase tracking-widest text-sepia-500 mb-0.5">Seeders</span>
						<span class="block text-sm {seedColor(selected.seeders)}">{selected.seeders}</span>
					</div>
					<div>
						<span class="block text-[10px] uppercase tracking-widest text-sepia-500 mb-0.5">Indexer</span>
						<span class="block text-sm text-sepia-800 dark:text-sepia-200">{selected.indexer}</span>
					</div>
				</div>
			</div>

			<div class="mt-4">
				{#if metaLoading}
					<div class="flex items-center gap-2 py-2 text-xs text-sepia-500">
						<Loader2 class="w-3 h-3 animate-spin" />
						Finding metadata…
					</div>
				{:else if metaSearchError || metaResults.length === 0}
					<!-- No results — silent -->
				{:else if metaSkipped}
					<div class="flex items-center justify-between">
						<p class="text-xs text-sepia-400">Using torrent title/author for folder naming.</p>
						<button
							type="button"
							class="text-xs text-sepia-600 underline underline-offset-2 hover:text-sepia-900 transition-colors dark:text-sepia-400 dark:hover:text-sepia-100"
							onclick={() => { metaSkipped = false; metaExpanded = true; }}
						>Add metadata</button>
					</div>
				{:else if !metaExpanded}
					<div class="rounded-lg bg-sepia-200 p-3 flex items-start justify-between gap-2 dark:bg-sepia-800">
						<div class="min-w-0">
							<div class="text-xs text-sepia-500 mb-1">BOOK METADATA</div>
							<div class="text-sm text-sepia-800 font-medium leading-snug truncate dark:text-sepia-200">{selectedMeta?.title}</div>
							<div class="text-xs text-sepia-600 mt-0.5 truncate dark:text-sepia-400">
								{selectedMeta?.author}{selectedMeta?.year ? ` · ${selectedMeta.year}` : ''}
							</div>
						</div>
						<Check class="w-4 h-4 text-green-700 shrink-0 mt-1 dark:text-green-400" />
					</div>
					<p class="text-xs text-sepia-400 mt-1.5">
						Looks wrong?
						<button type="button" class="text-sepia-600 underline underline-offset-2 hover:text-sepia-900 transition-colors dark:text-sepia-400 dark:hover:text-sepia-100" onclick={() => (metaExpanded = true)}>Change it</button>
						·
						<button type="button" class="text-sepia-600 underline underline-offset-2 hover:text-sepia-900 transition-colors dark:text-sepia-400 dark:hover:text-sepia-100" onclick={skipMeta}>Skip</button>
					</p>
				{:else}
					{#if !metaConfident}
						<p class="text-xs text-amber-700 mb-2 dark:text-amber-400">We're not confident these match — please pick one or skip.</p>
					{:else}
						<div class="text-xs text-sepia-500 mb-2">Pick the best match:</div>
					{/if}
					<div class="space-y-2">
						{#each metaResults as book (book.title + book.author)}
							{@const isSelected = selectedMeta === book}
							<button
								type="button"
								onclick={() => pickMeta(book)}
								class="w-full text-left rounded-lg p-3 transition-colors
									{isSelected ? 'bg-sepia-300 ring-1 ring-sepia-600 dark:bg-sepia-700 dark:ring-sepia-500' : 'bg-sepia-200 hover:bg-sepia-300 dark:bg-sepia-800 dark:hover:bg-sepia-700'}"
							>
								<div class="flex items-start justify-between gap-2">
									<div class="min-w-0">
										<div class="text-sm text-sepia-800 font-medium leading-snug truncate dark:text-sepia-200">{book.title}</div>
										<div class="text-xs text-sepia-600 mt-0.5 truncate dark:text-sepia-400">{book.author}</div>
									</div>
									<div class="flex items-center gap-2 shrink-0">
										{#if book.year}<span class="text-xs text-sepia-500">{book.year}</span>{/if}
										{#if isSelected}<Check class="w-3.5 h-3.5 text-sepia-700 dark:text-sepia-300" />{/if}
									</div>
								</div>
							</button>
						{/each}
					</div>
					<button
						type="button"
						class="mt-2 text-xs text-sepia-400 hover:text-sepia-700 transition-colors dark:hover:text-sepia-300"
						onclick={skipMeta}
					>None of these look right — skip</button>
				{/if}
			</div>

			<div class="flex gap-3 justify-end mt-4">
				<Dialog.Close>
					<Button variant="outline" disabled={requesting}>Cancel</Button>
				</Dialog.Close>
				<Button onclick={handleRequest} disabled={requesting}>
					{requesting ? 'Adding…' : 'Download'}
				</Button>
			</div>
		</Dialog.Content>
	</Dialog.Root>
{/if}

{#if toast}
	<div
		class="fixed bottom-6 right-6 z-[60] max-w-sm rounded-xl border px-4 py-3 text-sm shadow-xl transition-all {toast.type === 'success'
			? 'border-green-400 bg-green-50 text-green-800 dark:border-green-800 dark:bg-green-950 dark:text-green-300'
			: 'border-red-400 bg-red-50 text-red-800 dark:border-red-800 dark:bg-red-950 dark:text-red-300'}"
	>
		{toast.message}
	</div>
{/if}
