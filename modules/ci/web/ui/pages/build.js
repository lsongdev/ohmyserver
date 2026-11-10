import { html, useEffect, useRef, useState } from '@lsongdev/panel/html.js';
import { useAction } from '@lsongdev/panel/hooks.js';
import { PageHeader, Alert, Loading, LogView } from '@lsongdev/scripts/react/components/index.js';
import { usePanel } from '@lsongdev/panel/panel.js';
import { useApi, BuildStatus, EVENTS, active, datetime, short, duration } from '../ci.js';

export default function Build({ params }) {
  const { api, href, navigate } = usePanel();
  const build = useApi(`/builds/${params.id}`, b => (b && active(b.status) ? 1500 : 0));
  const repo = useApi(build.data ? `/repos/${build.data.repo_id}` : `/repos/0`);
  const action = useAction();
  const [chosen, setChosen] = useState(null);

  if (!build.data) return html`<${PageHeader} title="Build" back=${{ href: href('/'), label: 'Repositories' }} />${build.error ? html`<${Alert} error=${build.error} />` : html`<${Loading} />`}`;
  const b = build.data;
  const steps = b.steps || [];
  // Follow the running step until one is chosen; otherwise show the failing
  // or last step.
  const auto = steps.find(s => s.status === 'running') || steps.find(s => s.status === 'failure' || s.status === 'error')
    || [...steps].reverse().find(s => s.status !== 'skipped' && s.status !== 'pending') || steps[0];
  const step = steps.find(s => s.position === chosen) || auto;

  const cancel = () => action.run(async () => { await api.post(`/builds/${b.id}/cancel`); build.reload(); });
  const restart = () => action.run(async () => { const nb = await api.post(`/builds/${b.id}/restart`); navigate(`/builds/${nb.id}`); });

  return html`<${PageHeader} title=${`${repo.data?.name || ''} #${b.number}`}
    back=${{ href: href(`/repos/${b.repo_id}`), label: repo.data?.name || 'Repository' }}
    subtitle=${html`<${BuildStatus} status=${b.status} /> · <span class="mono">${b.branch}</span>${b.commit ? html` · <span class="commit">${short(b.commit)}</span>` : null}${
      ' · ' + [b.message, EVENTS[b.event] || b.event, b.author, datetime(b.created_at), duration(b.started_at, b.finished_at)].filter(Boolean).join(' · ')}`}
    actions=${active(b.status)
      ? html`<button class="button button-danger" disabled=${action.busy} onClick=${cancel}>Cancel</button>`
      : html`<button class="button" disabled=${action.busy} onClick=${restart}>Run again</button>`} />
    <${Alert} error=${action.error} />
    ${b.error ? html`<${Alert} variant=${b.status === 'canceled' ? 'warning' : 'danger'}>${b.error}<//>` : null}
    ${b.status === 'skipped' ? html`<${Alert} variant="info">The when condition in the configuration excludes this branch or event, so no steps ran.<//>` : null}
    ${b.status === 'pending' ? html`<${Alert} variant="info">Queued behind earlier builds.<//>` : null}
    ${steps.length ? html`<div class="build-layout">
      <div class="card" style="padding:0"><div class="step-list">
        ${steps.map(s => html`<button key=${s.position} class=${`step-item ${s === step ? 'active' : ''}`} onClick=${() => setChosen(s.position)}>
          <span class=${`dot ${s.status}`}></span><span class="name">${s.name}</span>
          <span class="time">${s.started_at ? duration(s.started_at, s.finished_at) : ''}</span>
        </button>`)}
      </div></div>
      <div class="build-log">${step ? html`<${StepLog} key=${`${b.id}-${step.position}`} build=${b.id} step=${step} />` : null}</div>
    </div>` : null}`;
}

// StepLog shows a step's log, appending to it while the step runs.
function StepLog({ build, step }) {
  const { api } = usePanel();
  const [text, setText] = useState('');
  const [error, setError] = useState(null);
  const offset = useRef(0);
  const running = step.status === 'running' || step.status === 'pending';
  useEffect(() => {
    let stopped = false;
    let timer;
    const load = async () => {
      try {
        const log = await api.get(`/builds/${build}/steps/${step.position}/log`, { query: { offset: offset.current } });
        if (stopped) return;
        if (log.text) setText(t => t + log.text);
        offset.current = log.offset;
        setError(null);
        if (!log.done) timer = setTimeout(load, 1000);
      } catch (err) {
        if (!stopped) { setError(err); timer = setTimeout(load, 3000); }
      }
    };
    load();
    return () => { stopped = true; clearTimeout(timer); };
  }, [build, step.position, running]);
  const lines = text ? text.replace(/\r\n?/g, '\n').replace(/\x1b\[[0-9;?]*[A-Za-z]/g, '').replace(/\n$/, '').split('\n') : [];
  return html`<div>
    <${Alert} error=${error} />
    <${LogView} ansi lines=${lines} empty=${step.status === 'skipped' ? 'Skipped' : running ? 'Waiting for output…' : 'No output'} />
  </div>`;
}
