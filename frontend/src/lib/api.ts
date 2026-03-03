let _token: string | null = null;

export function setToken(t: string): void {
	_token = t;
}

export function clearToken(): void {
	_token = null;
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
