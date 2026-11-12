// Firewall manager UI helpers shared by the pages.
import { html, useEffect, useRef, useState } from '@lsongdev/panel/html.js';
import { useAction, useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { PageHeader, Alert, toast } from '@lsongdev/scripts/react/components/index.js';

export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => (path ? api.get(path, { signal }) : Promise.resolve(undefined)), [path], { interval });
}

// Page shows a change on trial on top of every page, since the time left
// matters more than the page one happens to be on.
export function Page({ children, ...props }) {
  return html`<${PageHeader} ...${props} /><${TrialBanner} />${children}`;
}

const clock = s => `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;

const OUTCOMES = {
  kept: ['The new firewall is kept', 'success'],
  reverted: ['The previous firewall was restored', 'success'],
  expired: ['The change was not kept in time, so the previous firewall was restored', 'info'],
};

// TrialBanner shows the change that reverts unless it is kept. Losing the
// server is expected here: a firewall that is wrong can cut the panel off,
// and the previous one then comes back by itself.
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
      const [message, tone] = OUTCOMES[last.result];
      toast(message, tone);
      dispatchEvent(new Event('firewall:changed'));
    }
    if (status.data) seen.current = null;
  }, [trial?.deadline, status.data]);

  if (!trial) return null;
  const finish = path => action.run(async () => {
    await api.post(path);
    status.reload();
    dispatchEvent(new Event('firewall:changed'));
  });
  const lost = status.error?.code === 'network';
  return html`<div class="card trial-card" role="status">
    <div class="trial">
      <div class="trial-count" aria-label="Time left">${clock(left ?? trial.seconds)}</div>
      <div class="trial-text">
        <strong>The new firewall is on trial.</strong>
        <div class="muted small">It reverts by itself unless you keep it. Keeping it proves this connection still works.</div>
      </div>
      <div class="trial-actions">
        <button class="button button-primary" disabled=${action.busy || lost} onClick=${() => finish('/trial/keep')}>Keep firewall</button>
        <button class="button" disabled=${action.busy || lost} onClick=${() => finish('/trial/revert')}>Revert now</button>
      </div>
    </div>
    ${lost ? html`<div class="trial-note"><${Alert} variant="warning">The server cannot be reached from here. If the firewall cut you off, wait: the previous one comes back when the time is up.<//></div>` : null}
    ${trial.revert_error ? html`<div class="trial-note"><${Alert} variant="danger">Restoring failed and is being retried: ${trial.revert_error}<//></div>` : null}
    <${Alert} error=${action.error} />
  </div>`;
}

// useChanged reloads a resource when a trial ends or is kept.
export function useChanged(reload) {
  useEffect(() => {
    addEventListener('firewall:changed', reload);
    return () => removeEventListener('firewall:changed', reload);
  }, [reload]);
}
