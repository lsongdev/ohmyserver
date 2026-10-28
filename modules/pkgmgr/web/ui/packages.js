// Package manager UI helpers shared by the pages.
import { html } from './lib/html.js';
import { useResource } from './lib/hooks.js';
import { usePanel } from './lib/panel.js';
import { Notice, Loading, Status, toastError } from './lib/ui.js';

// useApi loads a GET endpoint, polling every interval(data) milliseconds
// while that returns a positive number. A null path loads nothing yet.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => (path ? api.get(path, { signal }) : Promise.resolve(undefined)), [path], { interval });
}

// useSystem loads the package manager and the running job, polling faster
// while a job runs.
export const useSystem = () => useApi('/system', s => (s?.running ? 2000 : 10000));

// useStartJob returns a function that starts a job and opens its page.
export function useStartJob() {
  const { api, navigate } = usePanel();
  return async request => {
    try {
      const job = await api.post('/jobs', request);
      navigate(`/jobs/${job.id}`);
    } catch (err) {
      toastError(err);
    }
  };
}

const JOB = { running: ['Running', 'blue'], succeeded: ['Succeeded', 'green'], failed: ['Failed', 'red'] };

export function JobStatus({ job }) {
  const [label, tone] = JOB[job.status] || [job.status, 'gray'];
  return html`<${Status} state=${job.status} label=${label} tone=${tone} title=${job.error} />`;
}

// RunningJob says which job is running; changes wait until it finishes.
export function RunningJob({ job }) {
  const { href } = usePanel();
  if (!job) return null;
  return html`<${Notice} tone="info">
    <strong>${job.title}</strong> is running. Other changes wait until it finishes. <a href=${href(`/jobs/${job.id}`)}>View output</a>
  <//>`;
}

// Unsupported explains a system without dnf, apt, pacman or brew.
export function Unsupported({ system }) {
  return html`<${Notice} tone="info">
    No supported package manager on ${system.os?.name || 'this system'}. Packages work with dnf
    (Fedora, RHEL, Rocky, AlmaLinux, Alibaba Cloud Linux, …), apt (Debian, Ubuntu), pacman (Arch Linux) and Homebrew (macOS).
  <//>`;
}

// PackageInfo expands above the list without blocking browsing or actions.
export function PackageInfo({ name, onClose }) {
  const info = useApi(`/info?name=${encodeURIComponent(name)}`);
  return html`<section class="card package-info" aria-label=${`Package information: ${name}`}>
    <div class="recipe-head"><h2>${name}</h2><button class="button button-small" type="button" onClick=${onClose}>Close details</button></div>
    ${info.data ? html`<pre class="info-text">${info.data.text || 'No information'}</pre>`
      : info.error ? html`<${Notice} error=${info.error} />` : html`<${Loading} />`}
  </section>`;
}

// duration formats how long a job ran, or has been running.
export function duration(job) {
  const end = job.finished_at ? new Date(job.finished_at) : new Date();
  const s = Math.max(0, Math.round((end - new Date(job.started_at)) / 1000));
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`;
}
