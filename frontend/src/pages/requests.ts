import { api } from '../api.js';
import { getUser, logout } from '../auth.js';
import { navigate } from '../router.js';

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
  // Present only in admin list view
  username?: string;
}

export function requestsPage(): HTMLElement {
  const user = getUser();
  const isAdmin = user?.role === 'admin';

  const root = el('div', 'min-h-screen bg-zinc-950 text-zinc-50');
  root.appendChild(buildNav());

  const main = el('main', 'mx-auto max-w-5xl px-4 py-8');
  root.appendChild(main);

  const heading = el('h1', 'text-2xl font-bold text-zinc-100 mb-6');
  heading.textContent = isAdmin ? 'All Requests' : 'My Requests';
  main.appendChild(heading);

  const tableArea = el('div', '');
  main.appendChild(tableArea);

  async function refresh(): Promise<void> {
    if (!root.isConnected) return;
    try {
      const data = await api.get<Request[]>('/api/requests');
      renderTable(tableArea, data, isAdmin);
    } catch (err: unknown) {
      renderError(tableArea, err instanceof Error ? err.message : 'Failed to load requests');
    }
  }

  renderLoading(tableArea);
  void refresh();

  const intervalId = setInterval(() => {
    if (!root.isConnected) {
      clearInterval(intervalId);
      return;
    }
    void refresh();
  }, 15_000);

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

  const searchBtn = document.createElement('button');
  searchBtn.className = 'text-sm text-zinc-400 hover:text-zinc-100 transition-colors';
  searchBtn.textContent = 'Search';
  searchBtn.addEventListener('click', () => void navigate('/'));
  right.appendChild(searchBtn);

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
    '</svg>Loading…';
  container.appendChild(loading);
}

function renderError(container: HTMLElement, message: string): void {
  container.replaceChildren();
  const err = el(
    'div',
    'rounded-lg border border-red-900 bg-red-950/40 px-4 py-3 text-sm text-red-400',
  );
  err.textContent = message;
  container.appendChild(err);
}

function renderTable(container: HTMLElement, requests: Request[], isAdmin: boolean): void {
  container.replaceChildren();

  if (requests.length === 0) {
    const empty = el('div', 'py-16 text-center text-sm text-zinc-500');
    empty.textContent = 'No requests yet. Head to Search to request an audiobook.';
    container.appendChild(empty);
    return;
  }

  const count = el('p', 'text-xs text-zinc-500 mb-3');
  count.textContent = `${requests.length} request${requests.length !== 1 ? 's' : ''}`;
  container.appendChild(count);

  const tableWrap = el('div', 'rounded-xl border border-zinc-800 overflow-hidden');
  container.appendChild(tableWrap);

  const table = document.createElement('table');
  table.className = 'w-full text-sm';
  tableWrap.appendChild(table);

  const thead = document.createElement('thead');
  thead.className = 'bg-zinc-900 border-b border-zinc-800';
  let thHtml =
    '<tr>' +
    '<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide">Title / Author</th>';
  if (isAdmin) {
    thHtml +=
      '<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide hidden sm:table-cell">User</th>';
  }
  thHtml +=
    '<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide">Status</th>' +
    '<th class="px-4 py-3 text-left text-xs font-medium text-zinc-400 uppercase tracking-wide hidden sm:table-cell">Submitted</th>' +
    '</tr>';
  thead.innerHTML = thHtml;
  table.appendChild(thead);

  const tbody = document.createElement('tbody');
  tbody.className = 'divide-y divide-zinc-800';
  table.appendChild(tbody);

  for (const req of requests) {
    const tr = document.createElement('tr');
    tr.className = 'bg-zinc-950 hover:bg-zinc-900 transition-colors';

    // Title / Author (+ error or path inline)
    const tdTitle = document.createElement('td');
    tdTitle.className = 'px-4 py-3';
    const titleDiv = el('div', 'font-medium text-zinc-100 leading-snug');
    titleDiv.textContent = req.title;
    tdTitle.appendChild(titleDiv);
    const authorDiv = el('div', 'text-xs text-zinc-400 mt-0.5');
    authorDiv.textContent = req.author;
    tdTitle.appendChild(authorDiv);
    if (req.status === 'failed' && req.error) {
      const errDiv = el('div', 'text-xs text-red-400 mt-1');
      errDiv.textContent = req.error;
      tdTitle.appendChild(errDiv);
    }
    if (req.status === 'done' && req.finalPath) {
      const pathDiv = el('div', 'text-xs text-zinc-500 mt-1 font-mono truncate max-w-xs');
      pathDiv.textContent = req.finalPath;
      tdTitle.appendChild(pathDiv);
    }
    tr.appendChild(tdTitle);

    // Username (admin only)
    if (isAdmin) {
      const tdUser = document.createElement('td');
      tdUser.className = 'px-4 py-3 text-xs text-zinc-400 hidden sm:table-cell';
      tdUser.textContent = req.username ?? '—';
      tr.appendChild(tdUser);
    }

    // Status badge
    const tdStatus = document.createElement('td');
    tdStatus.className = 'px-4 py-3';
    tdStatus.appendChild(statusBadge(req.status));
    tr.appendChild(tdStatus);

    // Submitted date
    const tdDate = document.createElement('td');
    tdDate.className = 'px-4 py-3 text-xs text-zinc-400 hidden sm:table-cell tabular-nums';
    tdDate.textContent = formatDate(req.createdAt);
    tr.appendChild(tdDate);

    tbody.appendChild(tr);
  }
}

function statusBadge(status: string): HTMLElement {
  const colors: Record<string, string> = {
    pending: 'bg-zinc-800 text-zinc-300',
    downloading: 'bg-blue-950 text-blue-300 border border-blue-800',
    moving: 'bg-amber-950 text-amber-300 border border-amber-800',
    done: 'bg-green-950 text-green-300 border border-green-800',
    failed: 'bg-red-950 text-red-300 border border-red-800',
  };
  const cls = colors[status] ?? 'bg-zinc-800 text-zinc-400';
  const badge = el('span', `inline-block rounded-full px-2.5 py-0.5 text-xs font-medium ${cls}`);
  badge.textContent = status;
  return badge;
}

function formatDate(iso: string): string {
  return new Date(iso).toLocaleDateString(undefined, {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
  });
}

function el(tag: string, className: string): HTMLElement {
  const e = document.createElement(tag);
  e.className = className;
  return e;
}
