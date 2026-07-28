/**
 * Common utility functions
 */
window.AppUtils = {
  /**
   * Format bytes to human readable string
   * @param {number} bytes - Bytes to format
   * @returns {string} Formatted string
   */
  formatBytes(bytes) {
    if (bytes === 0 || bytes === undefined) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  },

  /**
   * Format date string
   * @param {string} dateString - Date string to format
   * @returns {string} Formatted date string
   */
  formatDate(dateString) {
    if (!dateString) return '';
    const date = new Date(dateString);
    return date.toLocaleString();
  },

  /**
   * Show confirmation dialog
   * @param {string} message - Confirmation message
   * @returns {boolean} User confirmation result
   */
  confirm(message) {
    return window.confirm(message || 'Are you sure?');
  },

  /**
   * Handle method links (DELETE, PUT, etc.)
   * @param {Event} e - Click event
   */
  async handleMethodLink(e) {
    const target = e.target;
    if (target.tagName === 'A' && target.hasAttribute('method')) {
      e.preventDefault();
      const url = target.getAttribute('href');
      const method = target.getAttribute('method');
      const res = await fetch(url, {method});
      if(res.ok) {
        window.location.reload();
      } else {
        const text = await res.text();
        alert('Operation failed: ' + text);
      }

      // fetch(url, { method })
      //   .then(response => {
      //     if (!response.ok) {
      //       throw new Error('Network response was not ok');
      //     }
      //     window.location.reload();
      //   })
      //   .catch(error => {
      //     console.error('Error:', error);
      //     alert('Operation failed: ' + error.message);
      //   });
    }
  },

  /**
   * Initialize method link handlers
   */
  initMethodLinks() {
    document.addEventListener('click', (e) => this.handleMethodLink(e));
  }
};

// Auto-initialize on DOMContentLoaded
document.addEventListener('DOMContentLoaded', () => {
  AppUtils.initMethodLinks();
});
