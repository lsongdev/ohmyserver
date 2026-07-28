package server

import (
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/lsongdev/ohmyserver/forward"
)

// ProxyListView handles the TCP/UDP proxy list page.
func (s *Server) ProxyListView(w http.ResponseWriter, r *http.Request) {
	listeners, err := s.GetAllTCPListeners()
	if err != nil {
		s.Error(w, err)
		return
	}

	listenerList := make([]H, len(listeners))
	for i, l := range listeners {
		listenerList[i] = H{
			"ID":          l.ID,
			"Name":        l.Name,
			"Protocol":    l.Protocol,
			"ListenPort":  l.ListenPort,
			"ForwardHost": l.ForwardHost,
			"ForwardPort": l.ForwardPort,
			"Status":      l.Status,
			"CreatedAt":   l.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	s.Render(w, "forward/index", H{
		"listeners": listenerList,
	})
}

// ProxyNewHandler handles creating a TCP/UDP proxy listener.
func (s *Server) ProxyNewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.Render(w, "forward/new", H{})
		return
	}

	err := r.ParseForm()
	if err != nil {
		s.Error(w, err)
		return
	}

	name := r.FormValue("name")
	protocol := r.FormValue("protocol")
	listenPortStr := r.FormValue("listen_port")
	forwardHost := r.FormValue("forward_host")
	forwardPortStr := r.FormValue("forward_port")

	if name == "" || protocol == "" || listenPortStr == "" || forwardPortStr == "" {
		s.Error(w, &Error{Message: "Name, protocol, listen port, and forward port are required"})
		return
	}

	if protocol != "tcp" && protocol != "udp" {
		s.Error(w, &Error{Message: "Protocol must be tcp or udp"})
		return
	}

	listenPort, err := strconv.Atoi(listenPortStr)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid listen port"})
		return
	}

	forwardPort, err := strconv.Atoi(forwardPortStr)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid forward port"})
		return
	}

	if forwardHost == "" {
		forwardHost = "127.0.0.1"
	}

	listener, err := s.CreateTCPListener(name, protocol, listenPort, forwardHost, forwardPort)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Start TCP proxy
	if s.tcpProxy != nil && s.serviceDesiredEnabled("forward", true) {
		if err := s.validateServiceStart("forward"); err != nil {
			s.Error(w, err)
			return
		}
		config := &forward.TCPProxyConfig{
			ID:          listener.ID,
			Name:        listener.Name,
			Protocol:    listener.Protocol,
			ListenPort:  listener.ListenPort,
			ForwardHost: listener.ForwardHost,
			ForwardPort: listener.ForwardPort,
		}
		if err := s.tcpProxy.StartListener(config); err != nil {
			s.Error(w, err)
			return
		}
	}

	http.Redirect(w, r, "/forward", http.StatusSeeOther)
}

// ProxyEditHandler handles editing a TCP/UDP proxy listener.
func (s *Server) ProxyEditHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	if r.Method == http.MethodGet {
		listener, err := s.GetTCPListener(id)
		if err != nil {
			s.Error(w, err)
			return
		}

		s.Render(w, "forward/edit", H{
			"listener": listener,
		})
		return
	}

	err = r.ParseForm()
	if err != nil {
		s.Error(w, err)
		return
	}

	name := r.FormValue("name")
	protocol := r.FormValue("protocol")
	listenPortStr := r.FormValue("listen_port")
	forwardHost := r.FormValue("forward_host")
	forwardPortStr := r.FormValue("forward_port")

	if name == "" || protocol == "" || listenPortStr == "" || forwardPortStr == "" {
		s.Error(w, &Error{Message: "Name, protocol, listen port, and forward port are required"})
		return
	}

	if protocol != "tcp" && protocol != "udp" {
		s.Error(w, &Error{Message: "Protocol must be tcp or udp"})
		return
	}

	listenPort, err := strconv.Atoi(listenPortStr)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid listen port"})
		return
	}

	forwardPort, err := strconv.Atoi(forwardPortStr)
	if err != nil {
		s.Error(w, &Error{Message: "Invalid forward port"})
		return
	}

	err = s.UpdateTCPListener(id, name, protocol, listenPort, forwardHost, forwardPort)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Restart TCP proxy with new config
	if s.tcpProxy != nil && s.serviceDesiredEnabled("forward", true) {
		if err := s.validateServiceStart("forward"); err != nil {
			s.Error(w, err)
			return
		}
		// Stop and restart
		s.tcpProxy.StopListener(listenPort)
		config := &forward.TCPProxyConfig{
			ID:          id,
			Name:        name,
			Protocol:    protocol,
			ListenPort:  listenPort,
			ForwardHost: forwardHost,
			ForwardPort: forwardPort,
		}
		if err := s.tcpProxy.StartListener(config); err != nil {
			s.Error(w, err)
			return
		}
	}

	http.Redirect(w, r, "/forward", http.StatusSeeOther)
}

// ProxyDeleteHandler handles deleting a TCP/UDP proxy listener.
func (s *Server) ProxyDeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	// Stop TCP proxy
	listener, _ := s.GetTCPListener(id)
	if s.tcpProxy != nil && listener != nil {
		s.tcpProxy.StopListener(listener.ListenPort)
	}

	err = s.DeleteTCPListener(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	http.Redirect(w, r, "/forward", http.StatusSeeOther)
}

// ProxyDetailHandler handles the TCP/UDP proxy detail page.
func (s *Server) ProxyDetailHandler(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.Error(w, err)
		return
	}

	listener, err := s.GetTCPListener(id)
	if err != nil {
		s.Error(w, err)
		return
	}

	s.Render(w, "forward/detail", H{
		"listener": listener,
	})
}

// ProxyStatusHandler handles TCP/UDP proxy listener lifecycle actions.
func (s *Server) ProxyStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.URL.Query().Get("id")
	action := r.URL.Query().Get("action")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	_, err = s.GetTCPListener(id)
	if err != nil {
		http.Error(w, "Listener not found", http.StatusNotFound)
		return
	}

	switch action {
	case "activate":
		if err := s.UpdateTCPListenerStatus(id, "active"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Start TCP proxy
		if s.tcpProxy != nil && s.serviceDesiredEnabled("forward", true) {
			if err := s.validateServiceStart("forward"); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			listener, _ := s.GetTCPListener(id)
			if listener != nil {
				config := &forward.TCPProxyConfig{
					ID:          listener.ID,
					Name:        listener.Name,
					Protocol:    listener.Protocol,
					ListenPort:  listener.ListenPort,
					ForwardHost: listener.ForwardHost,
					ForwardPort: listener.ForwardPort,
				}
				if err := s.tcpProxy.StartListener(config); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
	case "deactivate":
		if err := s.UpdateTCPListenerStatus(id, "inactive"); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Stop TCP proxy
		if s.tcpProxy != nil {
			listener, _ := s.GetTCPListener(id)
			if listener != nil {
				s.tcpProxy.StopListener(listener.ListenPort)
			}
		}
	case "restart":
		if s.tcpProxy != nil && s.serviceDesiredEnabled("forward", true) {
			if err := s.validateServiceStart("forward"); err != nil {
				http.Error(w, err.Error(), http.StatusConflict)
				return
			}
			listener, _ := s.GetTCPListener(id)
			if listener != nil {
				s.tcpProxy.StopListener(listener.ListenPort)
				config := &forward.TCPProxyConfig{
					ID:          listener.ID,
					Name:        listener.Name,
					Protocol:    listener.Protocol,
					ListenPort:  listener.ListenPort,
					ForwardHost: listener.ForwardHost,
					ForwardPort: listener.ForwardPort,
				}
				if err := s.tcpProxy.StartListener(config); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
	default:
		http.Error(w, "Invalid action", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// startAllProxies starts all active TCP and UDP proxy listeners.
func (s *Server) startAllProxies() error {
	var errs []error
	// Start TCP proxies for all active TCP listeners
	tcpListeners, err := s.GetAllTCPListeners()
	if err != nil {
		log.Printf("Warning: Failed to get TCP listeners: %v\n", err)
		return err
	} else {
		for _, l := range tcpListeners {
			if l.Status == "active" {
				config := &forward.TCPProxyConfig{
					ID:          l.ID,
					Name:        l.Name,
					Protocol:    l.Protocol,
					ListenPort:  l.ListenPort,
					ForwardHost: l.ForwardHost,
					ForwardPort: l.ForwardPort,
				}
				if err := s.tcpProxy.StartListener(config); err != nil {
					log.Printf("Warning: Failed to start TCP proxy %s on port %d: %v\n", l.Name, l.ListenPort, err)
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}

func init() {
	RegisterInitHook(func(s *Server) error {
		s.tcpProxy = forward.NewTCPProxy()
		if !s.serviceDesiredEnabled("forward", true) {
			return nil
		}
		if err := s.validateServiceStart("forward"); err != nil {
			s.setServiceError("forward", err)
			return err
		}
		err := s.startAllProxies()
		s.setServiceError("forward", err)
		return err
	})
	RegisterCloseHook(func(s *Server) error {
		if s.tcpProxy != nil {
			s.tcpProxy.StopAllListeners()
		}
		return nil
	})
}
