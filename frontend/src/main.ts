import './style.css';
import { register, start } from './router.js';
import { loginPage } from './pages/login.js';
import { searchPage } from './pages/search.js';
import { getToken } from './api.js';

function guardAuth(): boolean {
  if (!getToken()) {
    window.location.replace('/login');
    return false;
  }
  return true;
}

register('/login', loginPage);

register('/', () => {
  if (!guardAuth()) return document.createElement('div');
  return searchPage();
});

register('/requests', () => {
  if (!guardAuth()) return document.createElement('div');
  // Implemented in step 11
  const placeholder = document.createElement('div');
  placeholder.className = 'min-h-screen bg-zinc-950 p-8 text-zinc-50';
  placeholder.textContent = 'My Requests — coming soon';
  return placeholder;
});

register('*', () => {
  window.location.replace('/login');
  return document.createElement('div');
});

void start();
