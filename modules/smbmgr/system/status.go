package system

import (
	"context"
	"encoding/json"
	"sort"
)

// Status is who is connected, from smbstatus --json (Samba 4.16 and later).
type Status struct {
	Sessions    []Session    `json:"sessions"`
	Connections []Connection `json:"connections"`
	OpenFiles   []OpenFile   `json:"open_files"`
}

// Session is a signed-in client.
type Session struct {
	ID         string `json:"id"`
	User       string `json:"user"`
	Group      string `json:"group"`
	Machine    string `json:"machine"`
	Dialect    string `json:"dialect"`
	Encryption string `json:"encryption"` // a cipher, or "-"
	Signing    string `json:"signing"`
	Since      string `json:"since"`
}

// Connection is a session using a share.
type Connection struct {
	Share   string `json:"share"`
	Session string `json:"session"`
	Machine string `json:"machine"`
	Since   string `json:"since"`
}

// OpenFile is a file a client holds open.
type OpenFile struct {
	Path   string `json:"path"`
	Share  string `json:"share"` // the share's path
	Name   string `json:"name"`  // relative to it
	Access string `json:"access"`
	Since  string `json:"since"`
	Opens  int    `json:"opens"`
}

// Status reads smbstatus.
func (s *Samba) Status(ctx context.Context) (Status, error) {
	out, _, err := s.Run.Run(ctx, "", "smbstatus", "--json")
	if err != nil {
		return Status{}, err
	}
	return parseStatus(out)
}

type cipher struct {
	Cipher string `json:"cipher"`
}

func parseStatus(out []byte) (Status, error) {
	var raw struct {
		Sessions map[string]struct {
			ID         string `json:"session_id"`
			Username   string `json:"username"`
			Groupname  string `json:"groupname"`
			Machine    string `json:"remote_machine"`
			Dialect    string `json:"session_dialect"`
			Created    string `json:"creation_time"`
			Encryption cipher `json:"encryption"`
			Signing    cipher `json:"signing"`
		} `json:"sessions"`
		Tcons map[string]struct {
			Service   string `json:"service"`
			SessionID string `json:"session_id"`
			Machine   string `json:"machine"`
			Connected string `json:"connected_at"`
		} `json:"tcons"`
		OpenFiles map[string]struct {
			ServicePath string `json:"service_path"`
			Filename    string `json:"filename"`
			Opens       map[string]struct {
				AccessMask struct {
					Text string `json:"text"`
				} `json:"access_mask"`
				OpenedAt string `json:"opened_at"`
			} `json:"opens"`
		} `json:"open_files"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return Status{}, err
	}
	st := Status{Sessions: []Session{}, Connections: []Connection{}, OpenFiles: []OpenFile{}}
	for _, s := range raw.Sessions {
		st.Sessions = append(st.Sessions, Session{
			ID: s.ID, User: s.Username, Group: s.Groupname, Machine: s.Machine, Dialect: s.Dialect,
			Encryption: s.Encryption.Cipher, Signing: s.Signing.Cipher, Since: s.Created,
		})
	}
	for _, t := range raw.Tcons {
		st.Connections = append(st.Connections, Connection{Share: t.Service, Session: t.SessionID, Machine: t.Machine, Since: t.Connected})
	}
	for path, f := range raw.OpenFiles {
		file := OpenFile{Path: path, Share: f.ServicePath, Name: f.Filename, Opens: len(f.Opens)}
		for _, o := range f.Opens {
			if file.Since == "" || o.OpenedAt < file.Since {
				file.Since = o.OpenedAt
			}
			if len(o.AccessMask.Text) > len(file.Access) {
				file.Access = o.AccessMask.Text
			}
		}
		st.OpenFiles = append(st.OpenFiles, file)
	}
	sort.Slice(st.Sessions, func(i, j int) bool { return st.Sessions[i].Since < st.Sessions[j].Since })
	sort.Slice(st.Connections, func(i, j int) bool { return st.Connections[i].Since < st.Connections[j].Since })
	sort.Slice(st.OpenFiles, func(i, j int) bool { return st.OpenFiles[i].Path < st.OpenFiles[j].Path })
	return st, nil
}
