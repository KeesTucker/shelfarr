import { api, setToken, clearToken } from './api';

export interface User {
	id: string;
	username: string;
	role: 'admin' | 'user';
}

class AuthStore {
	user = $state<User | null>(null);

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
