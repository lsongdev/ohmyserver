import { useEffect } from '@lsongdev/panel/html.js';
import { usePanel } from '@lsongdev/panel/panel.js';

function useRedirect(add) {
  const { navigate } = usePanel();
  useEffect(() => { navigate(add ? '/outbounds?addSubscription=1' : '/outbounds', { replace: true }); }, [add, navigate]);
  return null;
}

export default function SubscriptionListRedirect() {
  useRedirect(false);
  return null;
}

export function AddSubscriptionRedirect() {
  useRedirect(true);
  return null;
}
