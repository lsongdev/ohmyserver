// Network manager UI helpers shared by the pages.
import { html, useEffect, useRef, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { Page as BasePage, Notice, Status, toast } from '@lsongdev/panel/ui.js';

export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => (path ? api.get(path, { signal }) : Promise.resolve(undefined)), [path], { interval });
}

// Page is every page of this module: a change on trial shows on all of them,
// since the time left matters more than the page one happens to be on.
export function Page({ children, ...props }) {
  return html`<${BasePage} ...${props}><${TrialBanner} />${children}<//>`;
}

const clock = s => `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;

const OUTCOMES = {
  kept: i => [`The new settings of ${i} are kept`, 'success'],
  reverted: i => [`The settings of ${i} were restored`, 'success'],
  expired: i => [`The change to ${i} was not kept in time, so the previous settings were restored`, 'info'],
};

// TrialBanner shows the change that reverts unless it is kept. Losing the
// server is expected here, when the address changed or the change broke
// something, so the countdown carries on from what it last heard.
function TrialBanner() {
  const { api } = usePanel();
  const status = useResource(signal => api.get('/trial', { signal }), [], { interval: data => (data?.trial ? 2000 : 15000) });
  const action = useAction();
  const trial = status.data?.trial;
  const [left, setLeft] = useState(null);
  const seen = useRef(null);

  useEffect(() => { if (trial) setLeft(trial.seconds); }, [trial?.deadline, status.data]);
  useEffect(() => {
    if (!trial) return undefined;
    const timer = setInterval(() => setLeft(s => (s > 0 ? s - 1 : 0)), 1000);
    return () => clearInterval(timer);
  }, [trial?.deadline]);

  // Announce how a trial this page was watching ended.
  useEffect(() => {
    if (trial) { seen.current = trial.deadline; return; }
    const last = status.data?.last;
    if (seen.current && last) {
      const [message, tone] = OUTCOMES[last.result](last.interface);
      toast(message, tone);
      dispatchEvent(new Event('network:changed'));
    }
    if (status.data) seen.current = null;
  }, [trial?.deadline, status.data]);

  if (!trial) return null;
  const finish = path => action.run(async () => {
    await api.post(path);
    status.reload();
  });
  const lost = status.error?.code === 'network';
  return html`<div class="card trial-card" role="status">
    <div class="trial">
      <div class="trial-count" aria-label="Time left">${clock(left ?? trial.seconds)}</div>
      <div class="trial-text">
        <strong>New settings of ${trial.interface} are on trial.</strong>
        <div class="muted small">They revert by themselves unless you keep them. Keeping them proves this connection still works.</div>
      </div>
      <div class="trial-actions">
        <button class="button button-primary" disabled=${action.busy || lost} onClick=${() => finish('/trial/keep')}>Keep settings</button>
        <button class="button" disabled=${action.busy || lost} onClick=${() => finish('/trial/revert')}>Revert now</button>
      </div>
    </div>
    ${lost ? html`<div class="trial-note"><${Notice} tone="warn">The server cannot be reached from here. If you changed its address, open the panel at the new address and keep the settings there; otherwise wait, and the previous settings come back when the time is up.<//></div>` : null}
    ${trial.revert_error ? html`<div class="trial-note"><${Notice} tone="error">Restoring failed and is being retried: ${trial.revert_error}<//></div>` : null}
    <${Notice} error=${action.error} />
  </div>`;
}

// useChanged reloads a resource when a trial ends.
export function useChanged(reload) {
  useEffect(() => {
    addEventListener('network:changed', reload);
    return () => removeEventListener('network:changed', reload);
  }, [reload]);
}

const STATES = {
  up: ['running', 'Up'], unknown: ['running', 'Up'],
  down: ['stopped', 'Down'], dormant: ['waiting', 'Dormant'],
  lowerlayerdown: ['stopped', 'No link'],
};

// LinkStatus reads an interface's state as a person would say it.
export function LinkStatus({ link }) {
  if (!link.up) return html`<${Status} state="stopped" label="Off" />`;
  if (!link.carrier) return html`<${Status} state="waiting" label="No link" tone="amber" title="The interface is on, but has no link" />`;
  const [state, label] = STATES[link.state] || ['running', link.state];
  return html`<${Status} state=${state} label=${label} />`;
}

// rate formats bytes per second as network speeds are told: in bits.
export function rate(bytesPerSecond) {
  if (bytesPerSecond == null) return '—';
  let n = bytesPerSecond * 8;
  const units = ['bit/s', 'Kbit/s', 'Mbit/s', 'Gbit/s'];
  let i = 0;
  while (n >= 1000 && i < units.length - 1) { n /= 1000; i++; }
  return `${n < 10 && i > 0 ? n.toFixed(1) : Math.round(n)} ${units[i]}`;
}

export function speed(mbps) {
  if (!mbps) return null;
  return mbps >= 1000 ? `${mbps / 1000} Gbit/s` : `${mbps} Mbit/s`;
}

// useRates turns the counters of successive polls into speeds, per interface.
export function useRates(links) {
  const previous = useRef(new Map());
  const rates = useRef(new Map());
  const now = performance.now();
  for (const l of links || []) {
    const p = previous.current.get(l.name);
    if (p && (l.rx_bytes !== p.rx || l.tx_bytes !== p.tx || now - p.at > 1000)) {
      const seconds = (now - p.at) / 1000;
      if (seconds >= 0.5) {
        // Counters restart when a driver reloads.
        rates.current.set(l.name, {
          rx: Math.max(0, l.rx_bytes - p.rx) / seconds,
          tx: Math.max(0, l.tx_bytes - p.tx) / seconds,
        });
        previous.current.set(l.name, { rx: l.rx_bytes, tx: l.tx_bytes, at: now });
      }
    } else if (!p) {
      previous.current.set(l.name, { rx: l.rx_bytes, tx: l.tx_bytes, at: now });
    }
  }
  return rates.current;
}

// globalAddresses leaves out link-local addresses, which every IPv6
// interface has and which say little in a list.
export function globalAddresses(link) {
  return (link.addresses || []).filter(a => a.scope !== 'link');
}

export const cidr = a => `${a.address}/${a.prefix}`;
