import { api, setToken, clearToken } from './api.js';

export interface User {
  id: string;
  username: string;
  role: 'admin' | 'user';
}

let _user: User | null = null;

export function getUser(): User | null {
  return _user;
}

export async function login(username: string, password: string): Promise<void> {
  const data = await api.post<{ token: string; user: User }>('/api/auth/login', {
    username,
    password,
  });
  setToken(data.token);
  _user = data.user;
}

export function logout(): void {
  clearToken();
  _user = null;
  window.location.replace('/login');
}
