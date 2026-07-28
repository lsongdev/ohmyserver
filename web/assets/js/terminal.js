/**
 * Terminal Component
 * A reusable terminal component using xterm.js
 * Supports both WebSocket (interactive) and SSE (read-only logs) modes
 */
(function() {
    'use strict';

    // Terminal component class
    window.TerminalComponent = function(options) {
        this.options = Object.assign({
            containerId: 'terminal-container',
            elementId: 'terminal',
            connectingElementId: 'connecting',
            title: 'Terminal',
            showControls: true,
            wsUrl: '',
            sseUrl: '',
            sseEventName: 'log',
            host: '',
            port: '',
            fontSize: 14,
            fontFamily: 'Menlo, Monaco, "Courier New", monospace',
            theme: {
                background: '#1e1e1e',
                foreground: '#ffffff'
            },
            mode: 'ws', // 'ws' for WebSocket, 'sse' for SSE
            cursorBlink: true,
            disableStdin: false,
            height: '500px',
            onConnect: null,
            onDisconnect: null,
            onError: null
        }, options || {});

        this.term = null;
        this.fitAddon = null;
        this.ws = null;
        this.sse = null;
        this.initialized = false;
        this.sectionElement = null;
    };

    TerminalComponent.prototype = {
        // Initialize terminal
        init: function() {
            if (this.initialized) return;

            var self = this;

            // Create wrapper structure
            var containerElement = document.getElementById(this.options.containerId);
            if (!containerElement) {
                console.error('Terminal container element not found:', this.options.containerId);
                return;
            }

            // Create terminal section wrapper
            this.sectionElement = document.createElement('div');
            this.sectionElement.className = 'terminal-section';

            // Create header
            var headerElement = document.createElement('div');
            headerElement.className = 'terminal-header';

            // Create title
            var titleElement = document.createElement('h3');
            titleElement.textContent = this.options.title;
            headerElement.appendChild(titleElement);

            // Create controls
            if (this.options.showControls) {
                var controlsElement = document.createElement('div');
                controlsElement.className = 'terminal-controls';

                // Clear button
                var clearBtn = document.createElement('button');
                clearBtn.className = 'button button-small';
                clearBtn.innerHTML = '🧹 Clear';
                clearBtn.onclick = function() { self.clear(); };
                controlsElement.appendChild(clearBtn);

                // Fullscreen button
                var fullscreenBtn = document.createElement('button');
                fullscreenBtn.className = 'button button-small';
                fullscreenBtn.innerHTML = '⛶ Fullscreen';
                fullscreenBtn.onclick = function() { self.toggleFullscreen(); };
                controlsElement.appendChild(fullscreenBtn);

                headerElement.appendChild(controlsElement);
            }

            this.sectionElement.appendChild(headerElement);

            // Create terminal container
            var terminalContainer = document.createElement('div');
            terminalContainer.className = 'terminal-container';
            terminalContainer.style.height = this.options.height;
            terminalContainer.dataset.defaultHeight = this.options.height;

            // Create terminal element
            var terminalElement = document.createElement('div');
            terminalElement.id = this.options.elementId;
            terminalElement.className = 'terminal-element';
            terminalContainer.appendChild(terminalElement);

            // Create connecting overlay if needed
            if (this.options.mode === 'ws' && this.options.connectingElementId) {
                var connectingElement = document.createElement('div');
                connectingElement.id = this.options.connectingElementId;
                connectingElement.className = 'connecting-overlay';
                connectingElement.innerHTML = '<div class="spinner"></div><p>Connecting to server...</p>';
                terminalContainer.appendChild(connectingElement);
            }

            this.sectionElement.appendChild(terminalContainer);

            // Replace container content with our structure
            containerElement.innerHTML = '';
            containerElement.appendChild(this.sectionElement);

            // Create terminal instance
            this.term = new Terminal({
                cursorBlink: this.options.cursorBlink,
                fontSize: this.options.fontSize,
                fontFamily: this.options.fontFamily,
                theme: this.options.theme,
                convertEol: true,
                termName: 'xterm-256color',
                disableStdin: this.options.disableStdin
            });

            // Load fit addon
            this.fitAddon = new FitAddon.FitAddon();
            this.term.loadAddon(this.fitAddon);

            // Open terminal
            this.term.open(terminalElement);
            this.fitAddon.fit();

            // Focus after a small delay
            setTimeout(function() {
                if (self.term) {
                    self.term.focus();
                }
            }, 10);

            // Handle terminal data input (only for WebSocket mode)
            if (this.options.mode === 'ws') {
                this.term.onData(function(data) {
                    if (self.ws && self.ws.readyState === WebSocket.OPEN) {
                        self.ws.send(new TextEncoder().encode(data));
                    }
                });

                // Focus terminal on click
                terminalElement.addEventListener('pointerdown', function(e) {
                    if (self.term && self.term.element) {
                        self.term.focus();
                    }
                });
            }

            // Handle window resize (both modes)
            window.addEventListener('resize', function() {
                if (self.fitAddon && !self.options.disableAutoResize) {
                    self.fitAddon.fit();
                }
                if (self.options.mode === 'ws' && !self.options.disableSendSize) {
                    self.sendTerminalSize();
                }
            });

            this.initialized = true;
        },

        // Build WebSocket URL from options or use provided URL
        buildWsUrl: function() {
            if (this.options.wsUrl && this.options.wsUrl !== '<no value>') {
                return this.options.wsUrl;
            }

            // Build URL from browser location
            var protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
            var host = window.location.host;
            var path = this.options.path || '/terminal';

            return protocol + '//' + host + path;
        },

        // Connect to WebSocket or SSE
        connect: function(url) {
            if (this.options.mode === 'sse') {
                this.connectSSE(url);
            } else {
                this.connectWebSocket(url);
            }
        },

        // Connect to SSE (for logs)
        connectSSE: function(sseUrl) {
            var self = this;

            if (sseUrl) {
                this.options.sseUrl = sseUrl;
            }

            var url = this.options.sseUrl;
            if (!url || url.indexOf('<no value>') !== -1) {
                console.error('SSE URL not provided or invalid:', url);
                return;
            }

            var connecting = document.getElementById(this.options.connectingElementId);
            if (connecting) {
                connecting.classList.remove('hidden');
                connecting.innerHTML = '<p>Loading logs...</p>';
            }

            this.sse = new EventSource(url);

            this.sse.addEventListener(this.options.sseEventName, function(event) {
                if (connecting) {
                    connecting.classList.add('hidden');
                }
                self.term.writeln(event.data);
                self.term.scrollToBottom();
            });

            this.sse.addEventListener('error', function() {
                if (connecting) {
                    connecting.classList.add('hidden');
                }
                self.term.writeln('\r\n\x1b[31m[Error loading logs]\x1b[0m');
                if (typeof self.options.onError === 'function') {
                    self.options.onError();
                }
            });

            if (typeof self.options.onConnect === 'function') {
                self.options.onConnect();
            }
        },

        // Connect to WebSocket
        connectWebSocket: function(wsUrl) {
            var self = this;

            if (wsUrl) {
                this.options.wsUrl = wsUrl;
            }

            // Build or get WebSocket URL
            var url = this.buildWsUrl();

            if (!url || url.indexOf('<no value>') !== -1) {
                console.error('WebSocket URL not provided or invalid:', url);
                return;
            }

            if (this.ws && this.ws.readyState === WebSocket.OPEN) {
                this.term.focus();
                return;
            }

            var connecting = document.getElementById(this.options.connectingElementId);
            if (connecting) {
                connecting.classList.remove('hidden');
                connecting.innerHTML = '<div class="spinner"></div><p>Connecting to ' +
                    (this.options.host || this.options.port ? this.options.host + ':' + this.options.port : 'server') +
                    '...</p>';
            }

            this.connectTime = Date.now();
            this.ws = new WebSocket(url);
            this.ws.binaryType = 'arraybuffer';

            this.ws.onopen = function() {
                if (connecting) {
                    connecting.classList.add('hidden');
                }
                self.term.focus();
                self.sendTerminalSize();

                if (typeof self.options.onConnect === 'function') {
                    self.options.onConnect();
                }
            };

            this.ws.onmessage = function(event) {
                if (event.data instanceof ArrayBuffer) {
                    self.term.write(new Uint8Array(event.data));
                } else if (event.data instanceof Blob) {
                    var reader = new FileReader();
                    reader.onload = function() {
                        self.term.write(new Uint8Array(reader.result));
                    };
                    reader.readAsArrayBuffer(event.data);
                } else {
                    try {
                        var message = JSON.parse(event.data);
                        if (message.type === 'error') {
                            self.term.writeln('\r\n\x1b[31m[Error] ' + message.message + '\x1b[0m');
                        } else if (message.type === 'exit') {
                            self.term.writeln('\r\n\x1b[90m[Process exited]\x1b[0m');
                        }
                    } catch (_) {
                        self.term.write(event.data);
                    }
                }
            };

            this.ws.onclose = function() {
                self.term.writeln('\r\n\x1b[31m[Disconnected]\x1b[0m');
                if (typeof self.options.onDisconnect === 'function') {
                    self.options.onDisconnect();
                }
            };

            this.ws.onerror = function() {
                if (connecting) {
                    connecting.innerHTML = '<p style="color: #ff6b6b;">Connection failed</p><p>Check credentials and try again</p>';
                }
                if (typeof self.options.onError === 'function') {
                    self.options.onError();
                }
            };
        },

        // Send terminal size to server
        sendTerminalSize: function() {
            if (this.options.disableSendSize) return;
            if (this.ws && this.ws.readyState === WebSocket.OPEN && this.term) {
                this.ws.send(JSON.stringify({
                    type: 'resize',
                    cols: this.term.cols,
                    rows: this.term.rows
                }));
            }
        },

        // Clear terminal
        clear: function() {
            if (this.term) {
                this.term.clear();
                this.term.focus();
            }
        },

        // Toggle fullscreen
        toggleFullscreen: function() {
            var self = this;
            if (!this.sectionElement) return;

            var isFullscreen = this.sectionElement.classList.toggle('fullscreen');
            var terminalContainer = this.sectionElement.querySelector('.terminal-container');
            
            if (isFullscreen) {
                // Clear inline height to let CSS fullscreen rule take effect
                terminalContainer.style.height = '';
            } else {
                // Restore default height
                terminalContainer.style.height = terminalContainer.dataset.defaultHeight || this.options.height;
            }
            
            // Use requestAnimationFrame to ensure DOM updates before fit
            requestAnimationFrame(function() {
                requestAnimationFrame(function() {
                    if (self.fitAddon && !self.options.disableAutoResize) {
                        self.fitAddon.fit();
                    }
                    if (self.options.mode === 'ws' && !self.options.disableSendSize) {
                        self.sendTerminalSize();
                    }
                    self.term.focus();
                });
            });
        },

        // Disconnect
        disconnect: function() {
            if (this.ws) {
                this.ws.close();
                this.ws = null;
            }
            if (this.sse) {
                this.sse.close();
                this.sse = null;
            }
            if (this.latencyInterval) {
                clearInterval(this.latencyInterval);
                this.latencyInterval = null;
            }
        },

        // Destroy component
        destroy: function() {
            this.disconnect();
            if (this.term) {
                this.term.dispose();
                this.term = null;
            }
            this.initialized = false;
        }
    };
})();
