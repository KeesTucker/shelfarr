import { api } from './api';

export interface User {
	id: string;
	username: string;
	role: 'admin' | 'user';
}

class AuthStore {
	user = $state<User | null>(null);

	async login(username: string, password: string): Promise<void> {
		const data = await api.post<{ user: User }>('/api/auth/login', { username, password });
		this.user = data.user;
	}

	async logout(): Promise<void> {
		await api.post('/api/auth/logout', {}).catch(() => {});
		this.user = null;
		window.location.replace('/login');
	}

	async restore(): Promise<void> {
		try {
			this.user = await api.get<User>('/api/auth/me');
		} catch {
			this.user = null;
		}
	}

	get isAdmin(): boolean {
		return this.user?.role === 'admin';
	}
}

export const authStore = new AuthStore();
