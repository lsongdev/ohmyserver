import { render } from 'preact';
import { html } from '../shared/ui.js';
import { Shell } from './shell.js';

render(html`<${Shell} />`, document.getElementById('app'));
