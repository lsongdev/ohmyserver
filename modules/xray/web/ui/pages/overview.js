import { html } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { PageHeader, Alert, toast } from '@lsongdev/scripts/react/components/index.js';
import { WithConfig, entries, sections } from '../xray.js';

export default function Overview() {
  return html`<${WithConfig} render=${(state, reload) => html`<${Status} state=${state} reload=${reload} />`} />`;
}

function Status({ state, reload }) {
  const { api, href } = usePanel();
  const { busy, error, run } = useAction();
  const change = action => {
    if (state.running && !window.confirm(action === 'stop'
      ? 'Stop Xray? Active connections may be interrupted.'
      : 'Apply the saved configuration? Xray will restart and active connections may be interrupted.')) return;
    run(async () => {
      try {
        await api.post('/' + action, { revision: state.revision });
        toast(action === 'stop' ? 'Xray stopped' : 'Configuration applied', 'success');
      } finally {
        reload();
      }
    });
  };
  const actions = html`
      <button class="button button-primary" disabled=${busy || (state.running && !state.pending)}
        onClick=${() => change('apply')}>${state.running ? 'Apply changes' : 'Start Xray'}</button>
      <button class="button" disabled=${busy || !state.enabled} onClick=${() => change('stop')}>Stop Xray</button>`;
  return html`<${PageHeader} title="Xray" subtitle="Manage the embedded proxy and its routing configuration."
    actions=${actions} />
  <section class="xray-status">
    <h2>${state.running ? 'Running' : state.enabled ? 'Failed to start' : 'Stopped'}</h2>
    <p>${state.running
      ? (state.pending ? 'Saved changes are waiting to be applied.' : 'The saved configuration is active.')
      : 'Apply the saved configuration to start Xray.'}</p>
    <${Alert} error=${error || state.error} />
  </section>
  <ul class="xray-links">${Object.entries(sections).map(([key, section]) => html`
    <li key=${key}><a href=${href('/' + key)}>${section.title}</a> · ${entries(state.config, key).length}</li>`)}
    <li><a href=${href('/config')}>Edit full configuration</a></li>
  </ul>`;
}
