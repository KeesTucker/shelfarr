import './style.css';
import { register, start } from './router.js';
import { loginPage } from './pages/login.js';
import { searchPage } from './pages/search.js';
import { requestsPage } from './pages/requests.js';
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
  return requestsPage();
});

register('*', () => {
  window.location.replace('/login');
  return document.createElement('div');
});

void start();
