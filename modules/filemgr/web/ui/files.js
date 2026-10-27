// File manager UI helpers shared by the pages.
import { useResource } from './lib/hooks.js';
import { usePanel } from './lib/panel.js';

// useApi loads a GET endpoint, polling every interval(data) milliseconds
// while that returns a positive number. A null path loads nothing yet.
export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => (path ? api.get(path, { signal }) : Promise.resolve(undefined)), [path], { interval });
}

export const join = (dir, name) => (dir === '/' ? '' : dir) + '/' + name;
export const parent = p => (p === '/' ? '/' : p.slice(0, p.lastIndexOf('/')) || '/');
export const isDir = e => e.type === 'dir' || e.target_is === 'dir';

const IMAGES = /\.(png|jpe?g|gif|webp|avif|bmp|ico|svg)$/i;
export const isImage = name => IMAGES.test(name);

export function icon(e) {
  if (isDir(e)) return '▸';
  if (e.type === 'link') return '↪';
  if (isImage(e.name)) return '◩';
  return '▫';
}

// upload sends files to dir with progress (0..1), resolving with the API's
// answer; a conflict rejects with status 409.
export function upload(url, files, onProgress) {
  return new Promise((resolve, reject) => {
    const form = new FormData();
    for (const file of files) form.append('file', file, file.name);
    const xhr = new XMLHttpRequest();
    xhr.open('POST', url);
    xhr.upload.onprogress = e => e.lengthComputable && onProgress(e.loaded / e.total);
    xhr.onload = () => {
      let body = null;
      try { body = JSON.parse(xhr.responseText); } catch { /* not JSON */ }
      if (xhr.status >= 200 && xhr.status < 300) return resolve(body?.data);
      const err = new Error(body?.error?.message || `Upload failed (HTTP ${xhr.status})`);
      err.status = xhr.status;
      reject(err);
    };
    xhr.onerror = () => reject(new Error('Upload failed: the connection dropped'));
    xhr.send(form);
  });
}
