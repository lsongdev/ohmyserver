/**
 * LogViewer - Web component for displaying streaming logs with ANSI color support.
 *
 * Usage (HTML):
 *   <log-viewer id="viewer" dark></log-viewer>
 *
 * Usage (JS):
 *   const viewer = document.getElementById('viewer');
 *   viewer.connect('/api/stream');          // EventSource mode
 *   viewer.println('hello');                 // manual line with newline
 *   viewer.print('partial ');                // manual partial text
 *   viewer.printf('value: %s', 'test');     // printf-style
 *   viewer.clear();
 *   viewer.disconnect();
 */

// ANSI → CSS color map
const ANSI_COLORS = {
  // Normal
  30: '#000000', 31: '#cd3131', 32: '#0dbc79', 33: '#e5e510',
  34: '#2472c8', 35: '#bc3fbc', 36: '#11a8cd', 37: '#e5e5e5',
  // Bright
  90: '#666666', 91: '#f14c4c', 92: '#23d18b', 93: '#f5f543',
  94: '#3b8eea', 95: '#d670d6', 96: '#29b8db', 97: '#e5e5e5',
};

const ANSI_BG_COLORS = {
  // Normal
  40: '#000000', 41: '#cd3131', 42: '#0dbc79', 43: '#e5e510',
  44: '#2472c8', 45: '#bc3fbc', 46: '#11a8cd', 47: '#e5e5e5',
  // Bright
  100: '#666666', 101: '#f14c4c', 102: '#23d18b', 103: '#f5f543',
  104: '#3b8eea', 105: '#d670d6', 106: '#29b8db', 107: '#e5e5e5',
};

/** Convert an ANSI-escaped string to HTML spans. */
function ansiToHtml(text) {
  // Escape HTML entities first
  let escaped = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');

  // Replace ANSI escape sequences with HTML spans
  let result = '';
  let currentFg = null;
  let currentBg = null;
  let buffer = '';

  function flushBuffer() {
    if (!buffer) return;
    if (currentFg || currentBg) {
      let style = '';
      if (currentFg) style += `color:${currentFg};`;
      if (currentBg) style += `background:${currentBg};`;
      result += `<span style="${style}">${buffer}</span>`;
    } else {
      result += buffer;
    }
    buffer = '';
  }

  const regex = /\x1b\[([0-9;]*)m/g;
  let match;
  let lastIndex = 0;

  while ((match = regex.exec(escaped)) !== null) {
    // Flush text before this escape
    buffer += escaped.slice(lastIndex, match.index);
    lastIndex = regex.lastIndex;

    const codes = match[1] ? match[1].split(';').map(Number) : [0];

    for (const code of codes) {
      if (code === 0) {
        flushBuffer();
        currentFg = null;
        currentBg = null;
      } else if (code === 1) {
        flushBuffer();
        // Bold — we'll render via font-weight
        if (currentFg) currentFg = currentFg; // keep color, add bold later
      } else if (code in ANSI_COLORS) {
        flushBuffer();
        currentFg = ANSI_COLORS[code];
      } else if (code in ANSI_BG_COLORS) {
        flushBuffer();
        currentBg = ANSI_BG_COLORS[code];
      }
      // Ignore unsupported codes (reset handled above)
    }
  }

  buffer += escaped.slice(lastIndex);
  flushBuffer();
  return result;
}

class LogViewer extends HTMLElement {
  static get observedAttributes() {
    return ['dark'];
  }

  constructor() {
    super();
    this.attachShadow({ mode: 'open' });
    this.eventSource = null;
    this.currentLine = null;
  }

  connectedCallback() {
    this.shadowRoot.innerHTML = `
      <style>
        :host {
          display: block;
        }
        #logs {
          background: ${this.hasAttribute('dark') ? '#1e1e1e' : '#f5f5f5'};
          color: ${this.hasAttribute('dark') ? '#d4d4d4' : '#1a1a1a'};
          border: 1px solid ${this.hasAttribute('dark') ? '#333' : '#ddd'};
          border-radius: 4px;
          padding: 10px;
          margin: 10px 0;
          font-family: 'SF Mono', 'Cascadia Code', 'Fira Code', 'Menlo', monospace;
          font-size: 12px;
          line-height: 1.5;
        }
        #logs div {
          white-space: pre-wrap;
          word-wrap: break-word;
        }
        .line {
          min-height: 1.5em;
        }
      </style>
      <div id="logs"></div>
    `;
    this.content = this.shadowRoot.querySelector('#logs');

    if (this.hasAttribute('max-height')) {
      this.content.style.maxHeight = this.getAttribute('max-height');
      this.content.style.overflowY = 'auto';
    }

    if (this.hasAttribute('source')) {
      this.connect(this.getAttribute('source'));
    }
  }

  disconnectedCallback() {
    this.disconnect();
  }

  attributeChangedCallback(name, oldValue, newValue) {
    if (name === 'dark' && this.content) {
      const isDark = newValue !== null;
      this.content.style.background = isDark ? '#1e1e1e' : '#f5f5f5';
      this.content.style.color = isDark ? '#d4d4d4' : '#1a1a1a';
      this.content.style.borderColor = isDark ? '#333' : '#ddd';
    }
  }

  /** Print a complete line (adds newline). */
  println(text = '') {
    this.print(text);
    this.currentLine = null;
  }

  /** Print text without a trailing newline (subsequent print continues on same line). */
  print(text = '') {
    if (!this.currentLine) {
      this.currentLine = document.createElement('div');
      this.currentLine.classList.add('line');
      this.content.appendChild(this.currentLine);
    }
    this.currentLine.innerHTML = ansiToHtml(this.currentLine.innerHTML + text);
    this._autoScroll();
  }

  /** printf-style formatting. */
  printf(fmt, ...args) {
    let i = 0;
    const text = fmt.replace(/%s|%d|%v|%q/g, () => {
      return i < args.length ? String(args[i++]) : '';
    });
    this.print(text);
  }

  /** println with printf-style formatting. */
  printlnf(fmt, ...args) {
    let i = 0;
    const text = fmt.replace(/%s|%d|%v|%q/g, () => {
      return i < args.length ? String(args[i++]) : '';
    });
    this.println(text);
  }

  /** Clear all log content. */
  clear() {
    this.content.innerHTML = '';
    this.currentLine = null;
  }

  /**
   * Connect to an EventSource stream.
   * @param {string} source - URL for EventSource
   */
  connect(source) {
    this.disconnect();
    this.clear();
    this.eventSource = new EventSource(source);

    this.eventSource.addEventListener('log', (e) => {
      this.println(e.data);
    });

    this.eventSource.addEventListener('error', (e) => {
      this.printlnf('❌ ERROR: %s', e.data);
      this.disconnect();
    });

    this.eventSource.addEventListener('done', (e) => {
      this.printlnf('✅ %s', e.data);
      this.disconnect();
      this.dispatchEvent(new CustomEvent('completed'));
    });
  }

  /** Close the EventSource connection. */
  disconnect() {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
  }

  _autoScroll() {
    this.content.scrollTop = this.content.scrollHeight;
  }
}

customElements.define('log-viewer', LogViewer);
