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
      <button type="button" class="button button-primary" disabled=${busy || (state.running && !state.pending)}
        onClick=${() => change('apply')}>${state.running ? 'Apply changes' : 'Start Xray'}</button>
      <button type="button" class="button" disabled=${busy || !state.enabled} onClick=${() => change('stop')}>Stop Xray</button>`;
  const runtime = state.running ? 'Running' : state.enabled ? 'Failed to start' : 'Stopped';
  const runtimeTone = state.running ? 'success' : state.enabled ? 'danger' : 'neutral';
  const draft = state.running && !state.pending ? 'Applied'
    : state.enabled && state.pending ? 'Pending changes'
    : 'Ready to apply';
  const draftHint = state.running && !state.pending ? 'The running service uses this configuration.'
    : state.running ? 'Apply changes to update the running service.'
    : 'Start Xray to use the saved configuration.';

  return html`<${PageHeader} title="Xray" actions=${actions} />
  <section class="card xray-overview-status" aria-label="Service and configuration status">
    <dl class="xray-status-grid">
      <div>
        <dt>Service</dt>
        <dd><span class="xray-status-dot" data-tone=${runtimeTone} aria-hidden="true"></span>${runtime}</dd>
      </div>
      <div>
        <dt>Configuration</dt>
        <dd>${draft}<small>${draftHint}</small></dd>
      </div>
    </dl>
    ${(error || state.error) && html`<div class="xray-status-error"><${Alert} error=${error || state.error} /></div>`}
  </section>

  <section class="section" aria-labelledby="xray-saved-config">
    <div class="xray-summary-heading">
      <h2 id="xray-saved-config">Saved configuration</h2>
      <a href=${href('/config')}>Edit full JSON</a>
    </div>
    <div class="xray-summary-grid">
      ${Object.entries(sections).map(([key, section]) => html`
        <a key=${key} class="card xray-summary-card" href=${href('/' + key)}>
          <div class="xray-summary-label">
            <h3>${section.title}</h3>
            <span aria-hidden="true">→</span>
          </div>
          <strong class="stat-value">${entries(state.config, key).length}</strong>
        </a>`)}
    </div>
  </section>`;
}
