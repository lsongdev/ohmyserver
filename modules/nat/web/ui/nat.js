import { usePanel } from '@lsongdev/panel/panel.js';
import { useResource } from '@lsongdev/panel/hooks.js';

export function useApi(path, interval) {
  const { api } = usePanel();
  return useResource(signal => api.get(path, { signal }), [path], { interval });
}
export const mapping = {
  unknown: 'Unknown', inconsistent: 'Conflicting observations',
  endpoint_independent_observed: 'Stable across tested destinations',
  destination_dependent_observed: 'Varies by destination',
  endpoint_independent: 'Endpoint-independent',
  address_dependent: 'Address-dependent',
  address_port_dependent: 'Address and port-dependent',
};
export const filtering = {
  unknown: 'Unknown', endpoint_independent: 'Endpoint-independent',
  address_dependent: 'Address-dependent (inferred)', address_port_dependent: 'Address and port-dependent (inferred)',
};
export const types = {
  unknown: 'Undetermined', full_cone_like: 'Full cone-like',
  restricted_cone_like: 'Address-restricted cone-like', port_restricted_cone_like: 'Port-restricted cone-like',
  symmetric_like: 'Destination-dependent (symmetric-like)', no_translation_observed: 'No address translation observed',
};
export const statuses = { running: 'Running', completed: 'Completed', cancelled: 'Cancelled', timed_out: 'Time limit reached' };
export const date = value => value ? new Date(value).toLocaleString() : '—';
export const steps = {
  resolve: 'Resolve server', binding: 'Discover public mapping', behavior_baseline: 'Verify behavior server',
  mapping_alternate_ip: 'Mapping: alternate IP', mapping_alternate_ip_port: 'Mapping: alternate IP and port',
  filter_baseline: 'Filtering: fresh socket', filter_change_ip_port: 'Filtering: change IP and port',
  filter_change_port: 'Filtering: change port', filter_control_alternate: 'Control: alternate IP and port',
  filter_control_port: 'Control: alternate port',
};

// Explanations describe observations, without promising peer connectivity.
export function verdict(check) {
  const r = check.result;
  if (check.status === 'running') return { title: 'Discovering your network', description: 'Checking STUN connectivity, public mappings and incoming UDP replies.', tone: 'pending' };
  if (!r.udp) return { title: 'No STUN response received', description: 'A public endpoint could not be discovered. Routing, DNS, UDP filtering or server availability may be involved.', tone: 'warning' };
  const descriptions = {
    full_cone_like: 'The tested mapping stays stable, and replies from an alternate endpoint reached this server.',
    restricted_cone_like: 'The tested mapping stays stable. Replies appear to require prior traffic to the peer’s IP address.',
    port_restricted_cone_like: 'The tested mapping stays stable. Replies appear to require prior traffic to the peer’s exact IP address and port.',
    symmetric_like: 'The public mapping changes with the destination. Reusing a STUN endpoint for a different peer may not work.',
    no_translation_observed: 'The observed endpoint matches the local endpoint. Inbound firewall access still needs a separate test.',
  };
  if (descriptions[r.nat_type]) return { title: types[r.nat_type], description: descriptions[r.nat_type], tone: r.nat_type === 'symmetric_like' ? 'warning' : 'info' };
  if (r.mapping === 'inconsistent') return { title: 'Network observations differ', description: 'Servers reported conflicting mapping behavior. Review the evidence before assigning a NAT type.', tone: 'warning' };
  return { title: r.mapping === 'endpoint_independent_observed' ? 'Stable public mapping observed' : 'Public endpoint discovered', description: 'STUN is reachable, but there is not enough evidence to determine the full NAT behavior.', tone: 'info' };
}

export const mappingHint = {
  unknown: 'Not enough evidence to classify the mapping.',
  inconsistent: 'Results differ across tested destinations.',
  endpoint_independent_observed: 'The tested servers saw the same public endpoint.',
  destination_dependent_observed: 'The tested servers saw different public endpoints.',
  endpoint_independent: 'The public endpoint stays the same across tested destinations.',
  address_dependent: 'The public endpoint depends on the destination IP.',
  address_port_dependent: 'The public endpoint depends on the destination IP and port.',
};
export const filteringHint = {
  unknown: 'The server capabilities or replies were insufficient.',
  endpoint_independent: 'An alternate endpoint could send a reply without prior contact.',
  address_dependent: 'Prior contact with the peer’s IP appears necessary.',
  address_port_dependent: 'Prior contact with the peer’s exact IP and port appears necessary.',
};
export const elapsed = ms => ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`;
