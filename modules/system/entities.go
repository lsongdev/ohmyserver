package system

import "context"

type Info struct {
	ProgramName    string   `json:"program_name"`
	ProgramVersion string   `json:"program_version"`
	ProgramCommit  string   `json:"program_commit"`
	ProgramDesc    string   `json:"program_desc"`
	GoVersion      string   `json:"go_version"`
	BuildTime      string   `json:"build_time"`
	OS             string   `json:"os"`
	Arch           string   `json:"arch"`
	Listen         string   `json:"listen"`
	Database       string   `json:"database"`
	DockerMirror   string   `json:"docker_mirror"`
	Uptime         string   `json:"uptime"`
	NumCPU         int      `json:"num_cpu"`
	Credits        []Credit `json:"credits"`
}
type Credit struct {
	Name    string `json:"name"`
	URL     string `json:"url"`
	License string `json:"license"`
}
type Service interface {
	SystemInfo(context.Context) (Info, error)
}
