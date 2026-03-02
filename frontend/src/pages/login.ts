import { login } from '../auth.js';
import { navigate } from '../router.js';

export function loginPage(): HTMLElement {
  const root = el('div', 'min-h-screen flex items-center justify-center bg-zinc-950');
  const card = el('div', 'w-full max-w-sm rounded-xl border border-zinc-800 bg-zinc-900 p-8 shadow-2xl');
  root.appendChild(card);

  // Header
  const header = el('div', 'mb-6 text-center');
  header.innerHTML = `
    <div class="mb-3 text-4xl select-none">📚</div>
    <h1 class="text-2xl font-bold text-zinc-50">Bookarr</h1>
    <p class="mt-1 text-sm text-zinc-400">Sign in to continue</p>
    <p class="mt-1 text-sm text-zinc-400">Made with love & a lot of Claude for my partner</p>
  `;
  card.appendChild(header);

  // Form
  const form = document.createElement('form');
  form.className = 'space-y-4';
  card.appendChild(form);

  form.appendChild(labeledInput('username', 'Username', 'text', 'your username', 'username' as AutoFill));
  form.appendChild(labeledInput('password', 'Password', 'password', '••••••••', 'current-password' as AutoFill));

  // Error message
  const errorMsg = el('p', 'hidden text-sm text-red-400');
  form.appendChild(errorMsg);

  // Submit button
  const btn = document.createElement('button');
  btn.type = 'submit';
  btn.className =
    'mt-2 w-full rounded-lg bg-zinc-50 px-4 py-2.5 text-sm font-semibold text-zinc-900 ' +
    'hover:bg-zinc-200 focus:outline-none focus:ring-2 focus:ring-zinc-300 focus:ring-offset-2 ' +
    'focus:ring-offset-zinc-900 disabled:opacity-50 disabled:cursor-not-allowed transition-colors';
  btn.textContent = 'Sign in';
  form.appendChild(btn);

  form.addEventListener('submit', (e: SubmitEvent) => {
    e.preventDefault();
    const username = (form.elements.namedItem('username') as HTMLInputElement).value.trim();
    const password = (form.elements.namedItem('password') as HTMLInputElement).value;

    errorMsg.classList.add('hidden');
    errorMsg.textContent = '';
    btn.disabled = true;
    btn.textContent = 'Signing in…';

    login(username, password)
      .then(() => navigate('/'))
      .catch((err: unknown) => {
        errorMsg.textContent = err instanceof Error ? err.message : 'Login failed';
        errorMsg.classList.remove('hidden');
        btn.disabled = false;
        btn.textContent = 'Sign in';
      });
  });

  return root;
}

function el(tag: string, className: string): HTMLElement {
  const e = document.createElement(tag);
  e.className = className;
  return e;
}

function labeledInput(
  id: string,
  label: string,
  type: string,
  placeholder: string,
  autocomplete: AutoFill,
): HTMLDivElement {
  const wrapper = document.createElement('div');

  const lbl = document.createElement('label');
  lbl.htmlFor = id;
  lbl.className = 'block text-sm font-medium text-zinc-300 mb-1.5';
  lbl.textContent = label;

  const input = document.createElement('input');
  input.id = id;
  input.name = id;
  input.type = type;
  input.placeholder = placeholder;
  input.required = true;
  input.autocomplete = autocomplete;
  input.className =
    'w-full rounded-lg border border-zinc-700 bg-zinc-800 px-3 py-2.5 text-sm text-zinc-50 ' +
    'placeholder:text-zinc-500 focus:border-zinc-500 focus:outline-none focus:ring-1 ' +
    'focus:ring-zinc-500 transition-colors';

  wrapper.appendChild(lbl);
  wrapper.appendChild(input);
  return wrapper;
}
