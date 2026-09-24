
export function createAPI({ request, apiURL }) {
  return {
    listCertificates: (options = {}) => request(`/certificates`, options),
    getCertificate: (id, options = {}) => request(`/certificates/${encodeURIComponent(id)}`, options),
    listProviders: (options = {}) => request(`/providers`, options),
    listAccounts: (options = {}) => request(`/accounts`, options),
    listOrders: (options = {}) => request(`/orders`, options),
    listIssuances: (options = {}) => request(`/issuances`, options),
    getIssuance: (id, options = {}) => request(`/issuances/${encodeURIComponent(id)}`, options),
    listChallengeTargets: (options = {}) => request(`/challenge-targets`, options),
    startIssuance: (body, options = {}) => request(`/issuances`, { ...options, method: 'POST', body }),
    renewCertificate: (id, body, options = {}) => request(`/certificates/${encodeURIComponent(id)}/renew`, { ...options, method: 'POST', body }),
    createProvider: (body) => request(`/providers`, { method: 'POST', body }),
    generateCSR: (body) => request(`/certificates/csr`, { method: 'POST', body }),
    certificateDownloadURL: id => apiURL(`/certificates/${encodeURIComponent(id)}/download`),
    deleteProvider: (id) => request(`/providers/${encodeURIComponent(id)}`, { method: 'DELETE' }),
    registerAccount: (id, body) => request(`/providers/${encodeURIComponent(id)}/accounts`, { method: 'POST', body }),
    deleteAccount: (id) => request(`/accounts/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  };
}
