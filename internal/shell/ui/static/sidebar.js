import { html, useEffect, useRef } from '@lsongdev/panel/html.js';
import { Sidebar as SharedSidebar } from '@lsongdev/scripts/react/components/sidebar.js';

export function Sidebar({ storageKey, ...props }) {
  const root = useRef(null);
  useEffect(() => {
    const menu = root.current?.querySelector('.sidebar-nav > .menu-vertical');
    if (!menu || !storageKey) return;
    const key = `${storageKey}.scrollTop`;
    let position = 0;
    try { position = Math.max(0, Number(localStorage.getItem(key)) || 0); } catch { /* storage unavailable */ }

    let restored = position === 0;
    function restore() {
      if (restored || menu.clientHeight === 0 || menu.scrollHeight <= menu.clientHeight) return;
      menu.scrollTop = position;
      restored = true;
    }
    function remember() {
      if (!restored || menu.clientHeight === 0 || menu.scrollHeight <= menu.clientHeight) return;
      try { localStorage.setItem(key, String(menu.scrollTop)); } catch { /* storage unavailable */ }
    }
    const observer = new ResizeObserver(restore);
    observer.observe(menu);
    menu.addEventListener('scroll', remember, { passive: true });
    restore();
    return () => {
      observer.disconnect();
      menu.removeEventListener('scroll', remember);
    };
  }, [storageKey]);

  return html`<div class="layout-nav" ref=${root}><${SharedSidebar} storageKey=${storageKey} ...${props} /></div>`;
}
