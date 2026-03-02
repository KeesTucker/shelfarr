type RouteHandler = () => HTMLElement | Promise<HTMLElement>;

const routes = new Map<string, RouteHandler>();

export function register(path: string, handler: RouteHandler): void {
  routes.set(path, handler);
}

export async function navigate(path: string): Promise<void> {
  window.history.pushState({}, '', path);
  await render(path);
}

async function render(path: string): Promise<void> {
  const handler = routes.get(path) ?? routes.get('*');
  if (!handler) return;
  const el = await handler();
  document.getElementById('app')?.replaceChildren(el);
}

export function start(): Promise<void> {
  window.addEventListener('popstate', () => {
    void render(window.location.pathname);
  });
  return render(window.location.pathname);
}
