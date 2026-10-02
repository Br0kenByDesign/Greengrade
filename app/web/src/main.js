import './app.css';
import { mount } from 'svelte';
import App from './App.svelte';

const theme = localStorage.getItem('gg-theme');
if (theme === 'light' || theme === 'dark') document.documentElement.dataset.theme = theme;

mount(App, { target: document.getElementById('app') });
