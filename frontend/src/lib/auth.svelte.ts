import { api, setToken, clearToken, loadStoredToken } from './api';

export interface User {
	id: string;
	username: string;
	role: 'admin' | 'user';
}

/** Decode the user fields embedded in a JWT payload (no signature check). */
function userFromToken(token: string): User | null {
	try {
		const payload = JSON.parse(atob(token.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')));
		if (!payload.uid || !payload.username || !payload.role) return null;
		return { id: payload.uid, username: payload.username, role: payload.role };
	} catch {
		return null;
	}
}

class AuthStore {
	user = $state<User | null>(null);

	constructor() {
		const token = loadStoredToken();
		if (token) {
			this.user = userFromToken(token);
		}
	}

	async login(username: string, password: string): Promise<void> {
		const data = await api.post<{ token: string; user: User }>('/api/auth/login', {
			username,
			password,
		});
		setToken(data.token);
		this.user = data.user;
	}

	logout(): void {
		clearToken();
		this.user = null;
		window.location.replace('/login');
	}

	get isAdmin(): boolean {
		return this.user?.role === 'admin';
	}
}

export const authStore = new AuthStore();
