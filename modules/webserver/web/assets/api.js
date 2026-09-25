export function createAPI({ request } = {}) {
  return {
    listeners: ({signal}={}) => request(`/listeners`,{signal}),
    createListener: (item) => request(`/listeners`,{method:'POST',body:item}),
    updateListener: (id,item) => request(`/listeners/${id}`,{method:'PUT',body:item}),
    deleteListener: (id) => request(`/listeners/${id}`,{method:'DELETE'}),
    hosts: ({signal}={}) => request(`/hosts`,{signal}),
    policies: ({signal}={}) => request(`/policies`,{signal}),
    certificates: ({signal}={}) => request(`/certificates`,{signal}),
    createHost: item => request(`/hosts`,{method:'POST',body:item}),
    updateHost: (id,item) => request(`/hosts/${id}`,{method:'PUT',body:item}),
    deleteHost: id => request(`/hosts/${id}`,{method:'DELETE'}),
    createRoute: (hostID,item) => request(`/hosts/${hostID}/routes`,{method:'POST',body:item}),
    updateRoute: (id,item) => request(`/routes/${id}`,{method:'PUT',body:item}),
    deleteRoute: id => request(`/routes/${id}`,{method:'DELETE'}),
    createPolicy: item => request(`/policies`,{method:'POST',body:item}),
    updatePolicy: (id,item) => request(`/policies/${id}`,{method:'PUT',body:item}),
    deletePolicy: id => request(`/policies/${id}`,{method:'DELETE'}),
  };
}
