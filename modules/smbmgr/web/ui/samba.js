// Samba manager UI helpers shared by the pages.
import { html } from '@lsongdev/panel/html.js';
import { useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Alert, toast } from '@lsongdev/scripts/react/components/index.js';
import { Status } from '@lsongdev/panel/ui.js';

// useApi loads a GET endpoint, polling every interval milliseconds when
// that is positive. A null path loads nothing yet.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => (path ? api.get(path, { signal }) : Promise.resolve(undefined)), [path], { interval });
}

export function Unsupported() {
  return html`<${Alert} variant="info">Samba is not installed on this server. Install the <code>samba</code> package, then restart the panel.<//>`;
}

// ServiceStatus shows smbd's systemd state.
export function ServiceStatus({ service }) {
  if (!service) return html`<span class="muted">No systemd unit</span>`;
  if (service.active === 'active') return html`<${Status} state="running" label="Running" />`;
  if (service.active === 'failed') return html`<${Status} state="failed" label="Failed" />`;
  return html`<${Status} state="stopped" label=${service.active === 'inactive' ? 'Stopped' : service.active} />`;
}

// saved tells how a change to smb.conf went: written, and reloaded or not.
export function savedToast(message, saved) {
  if (saved?.reload_error) toast(`${message}, but smbd did not reload: ${saved.reload_error}`, 'danger');
  else toast(message, 'success');
  for (const w of saved?.warnings || []) toast(`testparm: ${w}`, 'info');
}

// canon is how Samba compares parameter names: case and spaces do not matter.
export const canon = key => key.toLowerCase().replace(/\s+/g, '');

// bool reads a Samba boolean, or returns fallback for anything else.
export function bool(value, fallback) {
  const v = String(value ?? '').toLowerCase();
  if (['yes', 'true', '1', 'on'].includes(v)) return true;
  if (['no', 'false', '0', 'off'].includes(v)) return false;
  return fallback;
}

// lookup finds a parameter by any of its names; the last one wins, as in
// Samba. With invert, the names in it mean the opposite (writeable for
// read only).
export function lookup(params, names, invert = []) {
  let found;
  for (const p of params) {
    const k = canon(p.key);
    if (names.includes(k)) found = { value: p.value, inverted: false };
    else if (invert.includes(k)) found = { value: p.value, inverted: true };
  }
  return found;
}

// ParamsEditor edits a list of { key, value } rows: the parameters a form
// has no field for.
export function ParamsEditor({ rows, onChange, disabled }) {
  const set = (i, patch) => onChange(rows.map((r, j) => (j === i ? { ...r, ...patch } : r)));
  return html`<div class="params">
    ${rows.length ? html`<div class="params-row params-head" aria-hidden="true"><span>Parameter</span><span>Value</span><span></span></div>` : null}
    ${rows.map((r, i) => html`<div class="params-row" key=${i}>
      <input class="input mono" aria-label="Parameter" placeholder="vfs objects" value=${r.key} disabled=${disabled}
        onInput=${e => set(i, { key: e.currentTarget.value })} />
      <input class="input mono params-value" aria-label=${`Value of ${r.key || 'parameter'}`} value=${r.value} disabled=${disabled}
        onInput=${e => set(i, { value: e.currentTarget.value })} />
      <button type="button" class="button button-ghost" aria-label=${`Remove ${r.key || 'parameter'}`} disabled=${disabled}
        onClick=${() => onChange(rows.filter((_, j) => j !== i))}>×</button>
    </div>`)}
    <div class="params-actions">
      <button type="button" class="button button-small" disabled=${disabled} onClick=${() => onChange([...rows, { key: '', value: '' }])}>Add parameter</button>
    </div>
  </div>`;
}
