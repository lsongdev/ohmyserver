/**
 * SSEClient - Fetch-based Server-Sent Events stream reader.
 *
 * Usage:
 *   const client = new SSEClient('/api/stream', {
 *     method: 'POST',
 *     body: formData,
 *     onEvent(eventType, data) { ... },
 *     onError(error) { ... },
 *     onDone() { ... },
 *   });
 *
 *   client.start();
 *   client.abort(); // cancel mid-stream
 */
class SSEClient {
  constructor(url, options = {}) {
    this.url = url;
    this.options = options;

    this.abortController = new AbortController();
    this.running = false;
  }

  /** Start streaming. Returns a promise that resolves when the stream ends. */
  async start() {
    this.running = true;

    const fetchOptions = {
      ...this.options,
      signal: this.abortController.signal,
      headers: {
        Accept: 'text/event-stream',
        ...(this.options.headers || {}),
      },
    };

    try {
      const response = await fetch(this.url, fetchOptions);

      if (!response.ok) {
        const errorText = await response.text();
        throw new Error(errorText || `HTTP ${response.status}`);
      }

      const reader = response.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';

      while (this.running) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });

        // Process complete SSE messages (separated by double newline)
        const parts = buffer.split('\n\n');
        buffer = parts.pop() || ''; // Keep incomplete message in buffer

        for (const part of parts) {
          const lines = part.split('\n');
          let eventType = 'message';
          let data = '';

          for (const line of lines) {
            if (line.startsWith('event: ')) {
              eventType = line.slice(7);
            } else if (line.startsWith('data: ')) {
              data = line.slice(6);
            }
          }

          if (eventType && data) {
            this._dispatch(eventType, data);
          }
        }
      }
    } catch (error) {
      if (error.name === 'AbortError') {
        this._dispatch('abort', 'Stream aborted');
      } else {
        if (typeof this.options.onError === 'function') {
          this.options.onError(error);
        }
      }
    } finally {
      this.running = false;
      if (typeof this.options.onDone === 'function') {
        this.options.onDone();
      }
    }
  }

  _dispatch(eventType, data) {
    if (typeof this.options.onEvent === 'function') {
      this.options.onEvent(eventType, data);
    }
  }

  /** Abort the active stream. */
  abort() {
    if (this.abortController) {
      this.abortController.abort();
      this.running = false;
    }
  }

  get isRunning() {
    return this.running;
  }
}
