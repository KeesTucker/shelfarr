import './style.css';
import { register, start } from './router.js';
import { loginPage } from './pages/login.js';
import { getToken } from './api.js';

register('/login', loginPage);

register('/', () => {
  if (!getToken()) {
    window.location.replace('/login');
    return document.createElement('div');
  }
  // Placeholder — replaced in step 10
  const el = document.createElement('div');
  el.className = 'min-h-screen bg-zinc-950 p-8 text-zinc-50';
  el.textContent = 'Welcome to Bookarr — search coming soon!';
  return el;
});

register('*', () => {
  window.location.replace('/login');
  return document.createElement('div');
});

void start();
