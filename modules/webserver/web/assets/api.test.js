import test from 'node:test';
import assert from 'node:assert/strict';
import { createAPI } from './api.js';

test('Web Server API maps relative CRUD endpoints for its host-scoped request transport', async () => {
  const calls = [];
  const request = async (path, options = {}) => {
    calls.push({ path, options });
    return null;
  };
  const api = createAPI({ request: (path, options) => request(`/v1/webserver${path}`, options) });
  const signal = new AbortController().signal;
  const listener = { name: 'public' };
  const host = { name: 'example' };
  const route = { name: 'api' };
  const policy = { name: 'edge' };

  await api.listeners({ signal });
  await api.createListener(listener);
  await api.updateListener(1, listener);
  await api.deleteListener(1);
  await api.hosts({ signal });
  await api.policies({ signal });
  await api.certificates({ signal });
  await api.createHost(host);
  await api.updateHost(2, host);
  await api.deleteHost(2);
  await api.createRoute(2, route);
  await api.updateRoute(3, route);
  await api.deleteRoute(3);
  await api.createPolicy(policy);
  await api.updatePolicy(4, policy);
  await api.deletePolicy(4);

  assert.deepEqual(calls.map(({ path, options }) => [path, options.method || 'GET']), [
    ['/v1/webserver/listeners', 'GET'],
    ['/v1/webserver/listeners', 'POST'],
    ['/v1/webserver/listeners/1', 'PUT'],
    ['/v1/webserver/listeners/1', 'DELETE'],
    ['/v1/webserver/hosts', 'GET'],
    ['/v1/webserver/policies', 'GET'],
    ['/v1/webserver/certificates', 'GET'],
    ['/v1/webserver/hosts', 'POST'],
    ['/v1/webserver/hosts/2', 'PUT'],
    ['/v1/webserver/hosts/2', 'DELETE'],
    ['/v1/webserver/hosts/2/routes', 'POST'],
    ['/v1/webserver/routes/3', 'PUT'],
    ['/v1/webserver/routes/3', 'DELETE'],
    ['/v1/webserver/policies', 'POST'],
    ['/v1/webserver/policies/4', 'PUT'],
    ['/v1/webserver/policies/4', 'DELETE'],
  ]);
  assert.equal(calls[0].options.signal, signal);
  assert.equal(calls[1].options.body, listener);
  assert.equal(calls[7].options.body, host);
  assert.equal(calls[10].options.body, route);
  assert.equal(calls[13].options.body, policy);
});

test('Web Server API supports an independent host-provided API base', async () => {
  const paths = [];
  const api = createAPI({ request: async path => { paths.push(`/custom/web${path}`); } });
  await api.listeners();
  await api.createRoute(9, { name: 'health' });
  assert.deepEqual(paths, ['/custom/web/listeners', '/custom/web/hosts/9/routes']);
});
