package host

import (
	"github.com/lsongdev/ohmyserver/internal/app"
	acmemodule "github.com/lsongdev/ohmyserver/modules/acme/web"
	clashmodule "github.com/lsongdev/ohmyserver/modules/clash/web"
	dashboardmodule "github.com/lsongdev/ohmyserver/modules/dashboard/web"
	disksmodule "github.com/lsongdev/ohmyserver/modules/disks"
	disksweb "github.com/lsongdev/ohmyserver/modules/disks/web"
	dnsweb "github.com/lsongdev/ohmyserver/modules/dns/web"
	dockerweb "github.com/lsongdev/ohmyserver/modules/docker/web"
	filesmodule "github.com/lsongdev/ohmyserver/modules/files"
	filesweb "github.com/lsongdev/ohmyserver/modules/files/web"
	forwardmodule "github.com/lsongdev/ohmyserver/modules/forward/web"
	monitormodule "github.com/lsongdev/ohmyserver/modules/monitor"
	monitorweb "github.com/lsongdev/ohmyserver/modules/monitor/web"
	openwrtweb "github.com/lsongdev/ohmyserver/modules/openwrt/web"
	packagesservice "github.com/lsongdev/ohmyserver/modules/packages/service"
	packagesmodule "github.com/lsongdev/ohmyserver/modules/packages/web"
	projectsweb "github.com/lsongdev/ohmyserver/modules/projects/web"
	pveweb "github.com/lsongdev/ohmyserver/modules/pve/web"
	settingsmodule "github.com/lsongdev/ohmyserver/modules/settings/web"
	speedtestweb "github.com/lsongdev/ohmyserver/modules/speedtest/web"
	systemmodule "github.com/lsongdev/ohmyserver/modules/system/web"
	terminalweb "github.com/lsongdev/ohmyserver/modules/terminal/web"
	uptimemodule "github.com/lsongdev/ohmyserver/modules/uptime"
	uptimeweb "github.com/lsongdev/ohmyserver/modules/uptime/web"
	webmodule "github.com/lsongdev/ohmyserver/modules/webserver/web"
	widgetsmodule "github.com/lsongdev/ohmyserver/modules/widgets/web"
	yeelightweb "github.com/lsongdev/ohmyserver/modules/yeelight/web"
)

// NewAPIModules is the composition root for feature APIs. Feature packages
// own their handlers and repositories; the host supplies capabilities and
// mounts modules under /v1/<name>.
func NewAPIModules(s *Server) (*app.Modules, error) {
	localFiles, err := filesmodule.NewLocalFilesystemAt("/")
	if err != nil {
		return nil, err
	}
	openWrtRepo := s.OpenWrtRepository()
	modules := []app.HTTPModule{
		dockerweb.New(s.DockerModuleService()),
		dashboardmodule.New(s.DashboardModuleService()),
		acmemodule.New(s.ACMEEngine()),
		settingsmodule.New(s.SettingsRepository()),
		s.localDHCPModule(),
		s.localNTPModule(),
		dnsweb.New(s.DNSRepository(), s.DNSModuleService(), s.DNSListDirectory()),
		uptimeweb.New(s.UptimeRepository(), uptimemodule.NewChecker(s.UptimeRepository())),
		forwardmodule.New(s.ForwardRepository(), s.ForwardModuleService()),
		widgetsmodule.New(s.WidgetsRepository()),
		webmodule.New(s.WebServerModuleService()),
		systemmodule.New(s.SystemModuleService()),
		projectsweb.New(s.ProjectsRepository(), s.ProjectDeploymentManager()),
		s.CronModule(),
		packagesmodule.New(packagesservice.NewSystemService()),
		disksweb.New(disksmodule.NewLocalService()),
		filesweb.New(localFiles),
		speedtestweb.New(speedtestweb.NewLocalService(s.SpeedtestRepository())),
		s.localIPMIModule(),
		s.localIDRACModule(),
		clashmodule.New(s.ClashService()),
		s.localESXiModule(),
		pveweb.New(pveweb.NewLocalService(s.PVERepository())),
		yeelightweb.New(yeelightweb.NewLocalService(s.YeelightRepository())),
		openwrtweb.New(openWrtRepo, openwrtweb.NewLocalService(openWrtRepo)),
		monitorweb.New(s.MonitorRepository(), monitormodule.LocalCollector{}),
		s.SSHModule(),
		terminalweb.New(s.TerminalModuleService()),
	}
	registry := app.NewModules()
	for _, module := range modules {
		if err := registry.Register(module); err != nil {
			return nil, err
		}
	}
	return registry, nil
}
