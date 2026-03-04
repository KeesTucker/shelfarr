interface ApiError {
	error: string;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
	const res = await fetch(path, {
		...init,
		credentials: 'same-origin',
		headers: { 'Content-Type': 'application/json', ...((init.headers as Record<string, string>) ?? {}) },
	});

	if (res.status === 401) {
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
