import { mount } from 'svelte';
import App from './App.svelte';
import MapWindow from './components/MapWindow.svelte';
import './app.css';

const target = document.getElementById('app');
if (!target) {
  throw new Error('index.html has no #app element');
}

// The same bundle is both windows. Go opens the map's at this hash
// (mapWindowURL in internal/app).
mount(location.hash === '#map' ? MapWindow : App, { target });
