import { html } from './ui.js';

export function requestPlaceholder({}) {
  return html`<div class="notice-card">
    <h2>模块迁移中</h2>
    <p>此模块已纳入新的模块导航，功能页面和 /v1 API 将按模块逐步迁移。</p>
  </div>`;
}
