const TOKEN_KEY = 'bookarr_token';

let _token: string | null = null;

/** Decode the exp claim from a JWT without verifying the signature. */
function jwtExpiry(token: string): number | null {
	try {
		const payload = JSON.parse(atob(token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')));
		return typeof payload.exp === 'number' ? payload.exp : null;
	} catch {
		return null;
	}
}

export function setToken(t: string): void {
	_token = t;
	localStorage.setItem(TOKEN_KEY, t);
}

export function clearToken(): void {
	_token = null;
	localStorage.removeItem(TOKEN_KEY);
}

/**
 * Restores a session from localStorage if a non-expired token exists.
 * Returns the stored token, or null if absent or expired.
 */
export function loadStoredToken(): string | null {
	const stored = localStorage.getItem(TOKEN_KEY);
	if (!stored) return null;
	const exp = jwtExpiry(stored);
	if (exp !== null && exp * 1000 < Date.now()) {
		localStorage.removeItem(TOKEN_KEY);
		return null;
	}
	_token = stored;
	return stored;
}

interface ApiError {
	error: string;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
	const headers: Record<string, string> = {
		'Content-Type': 'application/json',
		...((init.headers as Record<string, string>) ?? {}),
	};

	if (_token) {
		headers['Authorization'] = `Bearer ${_token}`;
	}

	const res = await fetch(path, { ...init, headers });

	if (res.status === 401) {
		clearToken();
		window.location.replace('/login');
		throw new Error('Unauthorized');
	}

	if (!res.ok) {
		const body = (await res.json().catch(() => ({ error: `HTTP ${res.status}` }))) as ApiError;
		throw new Error(body.error ?? `HTTP ${res.status}`);
	}

	return res.json() as Promise<T>;
}

export const api = {
	get<T>(path: string): Promise<T> {
		return request<T>(path);
	},
	post<T>(path: string, body: unknown): Promise<T> {
		return request<T>(path, { method: 'POST', body: JSON.stringify(body) });
	},
};
