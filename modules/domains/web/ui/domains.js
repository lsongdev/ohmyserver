// Domains UI helpers shared by the pages.
import { useResource } from '@lsongdev/panel/hooks.js';
import { usePanel } from '@lsongdev/panel/panel.js';

// useApi loads a GET endpoint; a null path loads nothing yet.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => (path ? api.get(path, { signal }) : Promise.resolve(undefined)), [path], { interval });
}

export const ttl = t => (!t || t === 1 ? 'Auto' : t >= 3600 && t % 3600 === 0 ? `${t / 3600} h` : t >= 60 && t % 60 === 0 ? `${t / 60} min` : `${t} s`);
