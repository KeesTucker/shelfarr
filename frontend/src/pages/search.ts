import { api } from '../api.js';
import { getUser, logout } from '../auth.js';
import { navigate } from '../router.js';

interface SearchResult {
  id: string;
  title: string;
  author: string;
  narrator?: string;
  size: number;
  seeders: number;
  indexer: string;
  publishDate?: string;
}

export function searchPage(): HTMLElement {
  const root = el('div', 'min-h-screen bg-zinc-950 text-zinc-50');
  root.appendChild(buildNav());

  const main = el('main', 'mx-auto max-w-5xl px-4 py-8');
  root.appendChild(main);

  const heading = el('h1', 'text-2xl font-bold text-zinc-100 mb-6');
  heading.textContent = 'Find an Audiobook';
  main.appendChild(heading);

  // Search input
  const inputWrapper = el('div', 'relative mb-6');
  main.appendChild(inputWrapper);

  const icon = el('span', 'absolute inset-y-0 left-0 flex items-center pl-3 pointer-events-none');
  icon.innerHTML =
    '<svg class="w-5 h-5 text-zinc-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">' +
    '<path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z"/>' +
    '</svg>';
  inputWrapper.appendChild(icon);

  const searchInput = document.createElement('input');
  searchInput.type = 'search';
  searchInput.placeholder = 'Search by title, author, narrator…';
  searchInput.className =
    'w-full rounded-lg border border-zinc-700 bg-zinc-900 pl-10 pr-4 py-3 text-sm text-zinc-50 ' +
    'placeholder:text-zinc-500 focus:border-zinc-500 focus:outline-none focus:ring-1 ' +
    'focus:ring-zinc-500 transition-colors';
  inputWrapper.appendChild(searchInput);

  const resultsArea = el('div', '');
  main.appendChild(resultsArea);

  let debounceId: ReturnType<typeof setTimeout> | null = null;

  function doSearch(q: string): void {
    if (q.length < 2) {
      resultsArea.replaceChildren();
      return;
    }
    renderLoading(resultsArea);
    api
      .get<SearchResult[]>(`/api/search?q=${encodeURIComponent(q)}`)
      .then((data) => renderResults(resultsArea, data))
      .catch((err: unknown) =>
        renderError(resultsArea, err instanceof Error ? err.message : 'Search failed'),
      );
  }

  searchInput.addEventListener('input', () => {
    if (debounceId !== null) clearTimeout(debounceId);
    debounceId = setTimeout(() => doSearch(searchInput.value.trim()), 400);
  });

  return root;
}

function buildNav(): HTMLElement {
  const user = getUser();
  const nav = el('nav', 'sticky top-0 z-10 border-b border-zinc-800 bg-zinc-900');
  const inner = el('div', 'mx-auto max-w-5xl px-4 h-14 flex items-center justify-between');
  nav.appendChild(inner);

  const brand = el('div', 'flex items-center gap-2 select-none');
  brand.innerHTML =
    '<span class="text-xl" aria-hidden="true">📚</span>' +
    '<span class="font-bold text-zinc-50">Bookarr</span>';
  inner.appendChild(brand);

  const right = el('div', 'flex items-center gap-4');
  inner.appendChild(right);

  const requestsBtn = document.createElement('button');
  requestsBtn.className = 'text-sm text-zinc-400 hover:text-zinc-100 transition-colors';
  requestsBtn.textContent = 'My Requests';
  requestsBtn.addEventListener('click', () => void navigate('/requests'));
  right.appendChild(requestsBtn);

  if (user) {
    const usernameEl = el('span', 'text-sm text-zinc-500 hidden sm:inline');
    usernameEl.textContent = user.username;
    right.appendChild(usernameEl);
  }

  const signOutBtn = document.createElement('button');
  signOutBtn.className =
    'rounded-lg border border-zinc-700 px-3 py-1.5 text-xs text-zinc-400 ' +
    'hover:border-zinc-500 hover:text-zinc-200 transition-colors';
  signOutBtn.textContent = 'Sign out';
  signOutBtn.addEventListener('click', logout);
  right.appendChild(signOutBtn);

  return nav;
}

function renderLoading(container: HTMLElement): void {
  container.replaceChildren();
  const loading = el('div', 'flex items-center justify-center gap-2 py-16 text-sm text-zinc-400');
  loading.innerHTML =
    '<svg class="animate-spin w-4 h-4" fill="none" viewBox="0 0 24 24">' +
    '<circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"/>' +
    '<path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4z"/>' +
    '</svg>Searching…';
  container.appendChild(loading);
}

function renderError(container: HTMLElement, message: string): void {
  container.replaceChildren();
  const err = el('div', 'rounded-lg border border-red-900 bg-red-950/40 px-4 py-3 text-sm text-red-400');
  err.textContent = message;
  container.appendChild(err);
}

function renderResults(container: HTMLElement, results: SearchResult[]): void {
  container.replaceChildren();

  if (results.length === 0) {
    const empty = el('div', 'py-16 text-center text-sm text-zinc-500');
    empty.textContent = 'No results found. Try a different search.';
    container.appendChild(empty);
    return;
  }

  const count = el('p', 'text-xs text-zinc-500 mb-3');
  count.textContent = `${results.length} result${results.length !== 1 ? 's' : ''}`;
  container.appendChild(count);

  const tableWrap = el('div', 'rounded-xl border border-zinc-800 overflow-hidden');
  container.appendChild(tableWrap);

  const table = document.createElement('table');
  table.className = 'w-full text-sm';
  tableWrap.appendChild(table);

  const thead = document.createElement('thead');
  thead.className = 'bg-zinc-900 border-b border-zinc-800';
  thead.innerHTML =
    '<tr>' +
    '<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide">Title / Author</th>' +
    '<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide hidden sm:table-cell">Narrator</th>' +
    '<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide hidden md:table-cell">Indexer</th>' +
    '<th class="px-4 py-3 text-right text-xs font-medium text-zinc-400 uppercase tracking-wide">Size</th>' +
    '<th class="px-4 py-3 text-right text-xs font-medium text-zinc-400 uppercase tracking-wide">Seeds</th>' +
    '</tr>';
  table.appendChild(thead);

  const tbody = document.createElement('tbody');
  tbody.className = 'divide-y divide-zinc-800';
  table.appendChild(tbody);

  for (const result of results) {
    const tr = document.createElement('tr');
    tr.className = 'bg-zinc-950 hover:bg-zinc-900 cursor-pointer transition-colors';

    const seedClass =
      result.seeders > 10 ? 'text-green-400' : result.seeders > 2 ? 'text-yellow-400' : 'text-red-400';

    const tdTitle = document.createElement('td');
    tdTitle.className = 'px-4 py-3';
    const titleDiv = el('div', 'font-medium text-zinc-100 leading-snug');
    titleDiv.textContent = result.title;
    const authorDiv = el('div', 'text-xs text-zinc-400 mt-0.5');
    authorDiv.textContent = result.author;
    tdTitle.appendChild(titleDiv);
    tdTitle.appendChild(authorDiv);
    tr.appendChild(tdTitle);

    const tdNarrator = document.createElement('td');
    tdNarrator.className = 'px-4 py-3 text-xs text-zinc-400 hidden sm:table-cell';
    tdNarrator.textContent = result.narrator || '—';
    tr.appendChild(tdNarrator);

    const tdIndexer = document.createElement('td');
    tdIndexer.className = 'px-4 py-3 hidden md:table-cell';
    const badge = el('span', 'inline-block rounded-full bg-zinc-800 px-2 py-0.5 text-xs text-zinc-300');
    badge.textContent = result.indexer;
    tdIndexer.appendChild(badge);
    tr.appendChild(tdIndexer);

    const tdSize = document.createElement('td');
    tdSize.className = 'px-4 py-3 text-right text-xs text-zinc-400 tabular-nums';
    tdSize.textContent = formatSize(result.size);
    tr.appendChild(tdSize);

    const tdSeeders = document.createElement('td');
    tdSeeders.className = `px-4 py-3 text-right text-xs font-medium tabular-nums ${seedClass}`;
    tdSeeders.textContent = String(result.seeders);
    tr.appendChild(tdSeeders);

    tr.addEventListener('click', () => showConfirmModal(result));
    tbody.appendChild(tr);
  }
}

function showConfirmModal(result: SearchResult): void {
  const overlay = el('div', 'fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4');
  document.body.appendChild(overlay);

  const card = el('div', 'w-full max-w-md rounded-xl border border-zinc-700 bg-zinc-900 p-6 shadow-2xl');
  overlay.appendChild(card);

  const h3 = el('h3', 'text-lg font-semibold text-zinc-50');
  h3.textContent = 'Confirm Download';
  card.appendChild(h3);

  const sub = el('p', 'text-sm text-zinc-400 mt-1 mb-5');
  sub.textContent = 'This will add the torrent to the download queue.';
  card.appendChild(sub);

  const meta = el('div', 'rounded-lg bg-zinc-800 p-4 space-y-3 mb-6');
  card.appendChild(meta);

  meta.appendChild(metaField('Title', result.title));
  meta.appendChild(metaField('Author', result.author));
  if (result.narrator) {
    meta.appendChild(metaField('Narrator', result.narrator));
  }

  const seedClass =
    result.seeders > 10 ? 'text-green-400' : result.seeders > 2 ? 'text-yellow-400' : 'text-red-400';

  const statsRow = el('div', 'flex gap-6');
  statsRow.appendChild(metaField('Size', formatSize(result.size)));
  statsRow.appendChild(metaField('Seeders', String(result.seeders), seedClass));
  statsRow.appendChild(metaField('Indexer', result.indexer));
  meta.appendChild(statsRow);

  const btnRow = el('div', 'flex gap-3 justify-end');
  card.appendChild(btnRow);

  const cancelBtn = document.createElement('button');
  cancelBtn.className =
    'rounded-lg border border-zinc-600 bg-zinc-800 px-4 py-2 text-sm text-zinc-300 ' +
    'hover:bg-zinc-700 transition-colors';
  cancelBtn.textContent = 'Cancel';
  btnRow.appendChild(cancelBtn);

  const downloadBtn = document.createElement('button');
  downloadBtn.className =
    'rounded-lg bg-zinc-50 px-4 py-2 text-sm font-semibold text-zinc-900 ' +
    'hover:bg-zinc-200 disabled:opacity-50 disabled:cursor-not-allowed transition-colors';
  downloadBtn.textContent = 'Download';
  btnRow.appendChild(downloadBtn);

  function close(): void {
    overlay.remove();
  }

  overlay.addEventListener('click', (e) => {
    if (e.target === overlay) close();
  });
  cancelBtn.addEventListener('click', close);

  downloadBtn.addEventListener('click', () => {
    downloadBtn.disabled = true;
    downloadBtn.textContent = 'Adding…';
    cancelBtn.disabled = true;
    api
      .post<{ id: string }>('/api/requests', {
        title: result.title,
        author: result.author,
        torrentGuid: result.id,
      })
      .then(() => {
        close();
        showToast(`"${result.title}" added to download queue`, 'success');
      })
      .catch((err: unknown) => {
        close();
        showToast(err instanceof Error ? err.message : 'Failed to add request', 'error');
      });
  });
}

function metaField(label: string, value: string, valueClass = 'text-zinc-200'): HTMLElement {
  const row = el('div', '');
  const lbl = el('span', 'block text-[10px] uppercase tracking-widest text-zinc-500 mb-0.5');
  lbl.textContent = label;
  const val = el('span', `block text-sm ${valueClass}`);
  val.textContent = value;
  row.appendChild(lbl);
  row.appendChild(val);
  return row;
}

function showToast(message: string, type: 'success' | 'error'): void {
  const toast = el(
    'div',
    'fixed bottom-6 right-6 z-[60] max-w-sm rounded-xl border px-4 py-3 text-sm shadow-2xl ' +
      (type === 'success'
        ? 'border-green-800 bg-green-950 text-green-300'
        : 'border-red-800 bg-red-950 text-red-300'),
  );
  toast.textContent = message;
  document.body.appendChild(toast);
  setTimeout(() => {
    toast.style.opacity = '0';
    toast.style.transform = 'translateY(0.5rem)';
    toast.style.transition = 'opacity 0.3s ease, transform 0.3s ease';
    setTimeout(() => toast.remove(), 350);
  }, 4000);
}

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '—';
  if (bytes >= 1_073_741_824) return `${(bytes / 1_073_741_824).toFixed(1)} GB`;
  if (bytes >= 1_048_576) return `${(bytes / 1_048_576).toFixed(0)} MB`;
  return `${Math.round(bytes / 1024)} KB`;
}

function el(tag: string, className: string): HTMLElement {
  const e = document.createElement(tag);
  e.className = className;
  return e;
}
