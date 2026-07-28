package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/lsongdev/ohmyserver/config"
	"github.com/lsongdev/ohmyserver/server"
	"github.com/lsongdev/ohmyserver/web"
)

type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := r.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("hijacking not supported")
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.statusCode,
			"duration", time.Since(start),
			"remote", r.RemoteAddr,
			"ua", r.UserAgent(),
		)
	})
}

func main() {
	initDefaultWeb := flag.Bool("init-default-web", false, "initialize the database and ensure a default :80 Worker host")
	flag.Parse()

	cfg, err := config.LoadConfig()
	if err != nil {
		panic(err)
	}

	server, err := server.NewServer(cfg)
	if err != nil {
		panic(err)
	}
	if *initDefaultWeb {
		if err := server.SeedDefaultWebWorker(); err != nil {
			_ = server.Close()
			log.Fatal(err)
		}
		if err := server.Close(); err != nil {
			log.Fatal(err)
		}
		return
	}
	router := http.NewServeMux()

	// Auth routes (not wrapped)
	router.HandleFunc("/login", server.LoginView)
	router.HandleFunc("/logout", server.LogoutHandler)
	router.HandleFunc("/hooks/github/{id}", server.GitHubWebhookHandler)

	// Static assets (from embedded FS)
	assetsFS, err := fs.Sub(web.Files, "assets")
	if err != nil {
		panic(err)
	}
	router.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.FS(assetsFS))))

	// Helper to wrap routes
	auth := server.AuthMiddleware

	router.HandleFunc("/", auth(server.IndexView))
	router.HandleFunc("/monitor", auth(server.MonitorView))
	router.HandleFunc("/monitor/stats", auth(server.StatsHandler))
	router.HandleFunc("/monitor/current", auth(server.CurrentStatsHandler))
	router.HandleFunc("/monitor/telegraf", server.TelegrafHandler)
	router.HandleFunc("/monitor/hosts/api/stats", auth(server.AllHostsStatsHandler))
	router.HandleFunc("/monitor/hosts/api/history", auth(server.HostHistoryHandler))
	router.HandleFunc("/monitor/hosts/{hostname}", auth(server.MonitorDetailView))
	router.HandleFunc("/monitor/uptime", auth(server.UptimeListView))
	router.HandleFunc("/monitor/uptime/new", auth(server.UptimeNewHandler))
	router.HandleFunc("/monitor/uptime/{id}", auth(server.UptimeDetailHandler))
	router.HandleFunc("/monitor/uptime/{id}/delete", auth(server.UptimeDeleteHandler))
	router.HandleFunc("/monitor/uptime/{id}/toggle", auth(server.UptimeToggleHandler))
	router.HandleFunc("/monitor/uptime/{id}/check", auth(server.UptimeCheckNowHandler))
	router.HandleFunc("/monitor/uptime/api/monitors", auth(server.UptimeAPIListHandler))
	router.HandleFunc("/monitor/uptime/api/monitors/{id}/checks", auth(server.UptimeAPIChecksHandler))
	router.HandleFunc("/docker/containers", auth(server.ContainerView))
	router.HandleFunc("/docker/containers/{id}", auth(server.ContainerActionHandler))
	router.HandleFunc("/docker/containers/create", auth(server.CreateContainer))
	router.HandleFunc("/docker/containers/{id}/terminal", auth(server.ContainerTerminalView))
	router.HandleFunc("/docker/containers/{id}/terminal/ws", auth(server.ContainerTerminal))
	router.HandleFunc("/docker/containers/{id}/logs", auth(server.ContainerLogsHandler))
	router.HandleFunc("/docker/containers/{id}/stats", auth(server.ContainerStatsHandler))
	router.HandleFunc("/docker/images", auth(server.ImageView))
	router.HandleFunc("/docker/images/pull", auth(server.PullImage))
	router.HandleFunc("/docker/images/build", auth(server.BuildImage))
	router.HandleFunc("/docker/images/{name}", auth(server.ImageDetailView))
	router.HandleFunc("/docker/volumes", auth(server.VolumeView))
	router.HandleFunc("/docker/volumes/create", auth(server.CreateVolume))
	router.HandleFunc("/docker/volumes/{name}", auth(server.VolumeDetailView))
	router.HandleFunc("/docker/networks", auth(server.NetworkView))
	router.HandleFunc("/docker/networks/create", auth(server.CreateNetwork))
	router.HandleFunc("/docker/networks/{id}", auth(server.NetworkDetailView))
	router.HandleFunc("/docker/compose", auth(server.ComposeView))
	router.HandleFunc("/docker/compose/new", auth(server.ComposeNewHandler))
	router.HandleFunc("/docker/compose/up", auth(server.ComposeUpSSEHandler))
	router.HandleFunc("/docker/compose/action", auth(server.ComposeActionHandler))
	router.HandleFunc("/docker/compose/{name}", auth(server.ComposeDetailView))
	router.HandleFunc("/apps", auth(server.AppsView))
	router.HandleFunc("/apps/sync", auth(server.AppsSyncHandler))
	router.HandleFunc("/apps/{id}", auth(server.AppDetailView))
	router.HandleFunc("/apps/{id}/icon", auth(server.AppIconHandler))
	router.HandleFunc("/apps/{id}/install", auth(server.AppInstallTriggerHandler))
	router.HandleFunc("/apps/{id}/action", auth(server.AppActionHandler))
	router.HandleFunc("/projects", auth(server.ProjectsView))
	router.HandleFunc("/projects/new", auth(server.NewProject))
	router.HandleFunc("/projects/{id}", auth(server.ProjectDetailView))
	router.HandleFunc("/projects/{id}/delete", auth(server.DeleteProjectHandler))
	router.HandleFunc("/projects/{id}/settings", auth(server.EditProject))
	router.HandleFunc("/projects/{id}/deployments", auth(server.DeploymentsView))
	router.HandleFunc("/projects/{id}/deployments/{deploymentId}", auth(server.DeploymentDetailView))
	router.HandleFunc("/projects/{id}/deployments/{deploymentId}/logs", auth(server.DeploymentLogsHandler))
	router.HandleFunc("/projects/{id}/deployments/{deploymentId}/cancel", auth(server.CancelDeploymentHandler))
	router.HandleFunc("/forward", auth(server.ProxyListView))
	router.HandleFunc("/forward/new", auth(server.ProxyNewHandler))
	router.HandleFunc("/forward/{id}", auth(server.ProxyDetailHandler))
	router.HandleFunc("/forward/{id}/edit", auth(server.ProxyEditHandler))
	router.HandleFunc("/forward/{id}/delete", auth(server.ProxyDeleteHandler))
	router.HandleFunc("/forward/status", auth(server.ProxyStatusHandler))
	router.HandleFunc("/proxy", auth(server.LegacyListenersRedirect))
	router.HandleFunc("/proxy/", auth(server.LegacyListenersRedirect))
	router.HandleFunc("/listeners", auth(server.LegacyListenersRedirect))
	router.HandleFunc("/listeners/", auth(server.LegacyListenersRedirect))
	router.HandleFunc("/domains", auth(server.LegacyDomainsRedirect))
	router.HandleFunc("/domains/", auth(server.LegacyDomainsRedirect))
	router.HandleFunc("/dns", auth(server.DNSView))
	router.HandleFunc("/dns/settings", auth(server.DNSSettingsHandler))
	router.HandleFunc("/dns/action", auth(server.DNSActionHandler))
	router.HandleFunc("/dns/zones", auth(server.DNSZonesView))
	router.HandleFunc("/dns/zones/new", auth(server.DNSZoneNewHandler))
	router.HandleFunc("/dns/zones/{id}", auth(server.DNSZoneDetailHandler))
	router.HandleFunc("/dns/zones/{id}/edit", auth(server.DNSZoneEditHandler))
	router.HandleFunc("/dns/zones/{id}/delete", auth(server.DNSZoneDeleteHandler))
	router.HandleFunc("/dns/zones/{id}/records/new", auth(server.DNSRecordNewHandler))
	router.HandleFunc("/dns/records/{id}/edit", auth(server.DNSRecordEditHandler))
	router.HandleFunc("/dns/records/{id}/delete", auth(server.DNSRecordDeleteHandler))
	router.HandleFunc("/dns/providers", auth(server.DNSProvidersView))
	router.HandleFunc("/dns/providers/new", auth(server.DNSProviderNewHandler))
	router.HandleFunc("/dns/providers/{id}", auth(server.DNSProviderHandler))
	router.HandleFunc("/dns/providers/{id}/zones/{zone}", auth(server.DNSProviderZoneHandler))
	router.HandleFunc("/dns/filters", auth(server.DNSFiltersView))
	router.HandleFunc("/dns/filters/subscriptions/new", auth(server.DNSSubscriptionNewHandler))
	router.HandleFunc("/dns/filters/subscriptions/{id}/edit", auth(server.DNSSubscriptionEditHandler))
	router.HandleFunc("/dns/filters/subscriptions/{id}/action", auth(server.DNSSubscriptionActionHandler))
	router.HandleFunc("/dns/filters/rules", auth(server.DNSRuleNewHandler))
	router.HandleFunc("/dns/filters/rules/{id}/action", auth(server.DNSRuleActionHandler))
	router.HandleFunc("/ntp", auth(server.NTPView))
	router.HandleFunc("/ntp/settings", auth(server.NTPSettingsHandler))
	router.HandleFunc("/ntp/action", auth(server.NTPActionHandler))
	router.HandleFunc("/dhcp", auth(server.DHCPView))
	router.HandleFunc("/dhcp/settings", auth(server.DHCPSettingsHandler))
	router.HandleFunc("/dhcp/action", auth(server.DHCPActionHandler))
	router.HandleFunc("/dhcp/leases", auth(server.DHCPLeaseListView))
	router.HandleFunc("/dhcp/static-leases", auth(server.DHCPStaticLeaseListView))
	router.HandleFunc("/dhcp/static-leases/new", auth(server.DHCPStaticLeaseNewHandler))
	router.HandleFunc("/dhcp/static-leases/{id}", auth(server.DHCPStaticLeaseDetailHandler))
	router.HandleFunc("/dhcp/static-leases/{id}/edit", auth(server.DHCPStaticLeaseEditHandler))
	router.HandleFunc("/dhcp/leases/{id}", auth(server.DHCPLeaseDeleteHandler))
	router.HandleFunc("/cron", auth(server.CronJobView))
	router.HandleFunc("/cron/new", auth(server.CronJobNewHandler))
	router.HandleFunc("/cron/{id}", auth(server.CronJobDetailHandler))
	router.HandleFunc("/cron/{id}/edit", auth(server.CronJobEditHandler))
	router.HandleFunc("/cron/{id}/delete", auth(server.CronJobDeleteHandler))
	router.HandleFunc("/cron/{id}/toggle", auth(server.CronJobToggleHandler))
	router.HandleFunc("/cron/{id}/run", auth(server.CronJobRunNowHandler))
	router.HandleFunc("/ssh/servers", auth(server.SSHListView))
	router.HandleFunc("/ssh/servers/new", auth(server.SSHNewView))
	router.HandleFunc("/ssh/servers/{id}", auth(server.SSHDetailView))
	router.HandleFunc("/ssh/servers/{id}/edit", auth(server.SSHEditView))
	router.HandleFunc("/ssh/servers/{id}/terminal", auth(server.SSHTerminalView))
	router.HandleFunc("/ssh/servers/{id}/terminal/ws", auth(server.SSHTerminal))
	router.HandleFunc("/ssh/servers/{id}/stats", auth(server.SSHStatsHandler))
	router.HandleFunc("/ssh/servers/{id}/metrics", auth(server.SSHMetricsHandler))
	router.HandleFunc("/ssh/servers/test", auth(server.SSHTestConnection))
	router.HandleFunc("/terminal", auth(server.TerminalHandler))
	router.HandleFunc("/terminal/local/ws", auth(server.TerminalWS))
	router.HandleFunc("/esxi", auth(server.ESXiServersView))
	router.HandleFunc("/esxi/new", auth(server.ESXiServerNewHandler))
	router.HandleFunc("/esxi/{id}", auth(server.ESXiServerDetailHandler))
	router.HandleFunc("/esxi/{id}/edit", auth(server.ESXiServerEditHandler))
	router.HandleFunc("/pve", auth(server.PVEListView))
	router.HandleFunc("/pve/new", auth(server.PVENewHandler))
	router.HandleFunc("/pve/{id}", auth(server.PVEDetailHandler))
	router.HandleFunc("/pve/{id}/edit", auth(server.PVEEditHandler))
	router.HandleFunc("/pve/{id}/instances/new", auth(server.PVECreateInstanceHandler))
	router.HandleFunc("/acme/certificates", auth(server.CertificatesView))
	router.HandleFunc("/acme/certificates/new", auth(server.ACMECertificateNewHandler))
	router.HandleFunc("/acme/certificates/{id}", auth(server.CertificateDetailView))
	router.HandleFunc("/acme/certificates/{id}/download", auth(server.CertificateDownloadHandler))
	router.HandleFunc("/acme/create-csr", auth(server.CreateCSRView))
	router.HandleFunc("/acme/providers", auth(server.ACMEProvidersView))
	router.HandleFunc("/acme/providers/new", auth(server.ACMEProviderNewView))
	router.HandleFunc("/acme/providers/{id}", auth(server.ACMEProvidersView))
	router.HandleFunc("/acme/providers/{id}/register", auth(server.ACMERegisterView))
	router.HandleFunc("/acme/accounts", auth(server.ACMEAccountsView))
	router.HandleFunc("/acme/accounts/{id}", auth(server.ACMEAccountDeleteHandler))
	router.HandleFunc("/acme/orders", auth(server.ACMEOrdersView))
	router.HandleFunc("/acme/orders/new", auth(server.ACMEOrderNewView))
	router.HandleFunc("/acme/orders/{id}", auth(server.ACMEOrderDetailHandler))
	router.HandleFunc("/acme/orders/{id}/finalize", auth(server.ACMEOrderFinalize))
	router.HandleFunc("/acme/challenges/complete", auth(server.ACMEChallengeComplete))
	router.HandleFunc("/acme/issuances/{id}", auth(server.ACMEIssuanceView))
	router.HandleFunc("/clash", auth(server.ClashIndexView))
	router.HandleFunc("/clash/new", auth(server.ClashNewHandler))
	router.HandleFunc("/clash/{id}", auth(server.ClashDetailHandler))
	router.HandleFunc("/clash/{id}/test", auth(server.ClashTestConnectionHandler))
	router.HandleFunc("/clash/{id}/action", auth(server.ClashProxyActionHandler))
	router.HandleFunc("/clash/{id}/proxies", auth(server.ClashProxiesView))
	router.HandleFunc("/clash/{id}/rules", auth(server.ClashRulesView))
	router.HandleFunc("/clash/{id}/connections", auth(server.ClashConnectionsView))
	router.HandleFunc("/clash/{id}/stream", auth(server.ClashStreamSSE))
	router.HandleFunc("/clash/{id}/settings", auth(server.ClashEditHandler))
	router.HandleFunc("/clash/{id}/config", auth(server.ClashConfigHandler))
	router.HandleFunc("/files", auth(server.FileView))
	router.HandleFunc("/files/new", auth(server.FileNew))
	router.HandleFunc("/files/download", auth(server.FileDownload))
	router.HandleFunc("/widgets", auth(server.WidgetListView))
	router.HandleFunc("/widgets/new", auth(server.WidgetNewHandler))
	router.HandleFunc("/widgets/{id}", auth(server.WidgetDetailHandler))
	router.HandleFunc("/widgets/{id}/edit", auth(server.WidgetEditHandler))
	router.HandleFunc("/settings", auth(server.SettingsListView))
	router.HandleFunc("/settings/new", auth(server.SettingsNewHandler))
	router.HandleFunc("/settings/{id}", auth(server.SettingsEditHandler))
	router.HandleFunc("/about", auth(server.AboutView))
	router.HandleFunc("/ipmi", auth(server.IPMIView))
	router.HandleFunc("/ipmi/new", auth(server.IPMINewHandler))
	router.HandleFunc("/ipmi/{id}", auth(server.IPMIDetailHandler))
	router.HandleFunc("/ipmi/{id}/edit", auth(server.IPMIEditHandler))
	router.HandleFunc("/ipmi/{id}/power", auth(server.IPMIPowerControlHandler))
	router.HandleFunc("/ipmi/{id}/test", auth(server.IPMITestConnectionHandler))
	router.HandleFunc("/idrac", auth(server.IDRACListView))
	router.HandleFunc("/idrac/new", auth(server.IDRACNewHandler))
	router.HandleFunc("/idrac/{id}", auth(server.IDRACDetailHandler))
	router.HandleFunc("/idrac/{id}/edit", auth(server.IDRACEditHandler))
	router.HandleFunc("/idrac/{id}/power", auth(server.IDRACPowerControlHandler))
	router.HandleFunc("/openwrt", auth(server.OpenWrtListView))
	router.HandleFunc("/openwrt/new", auth(server.OpenWrtNewHandler))
	router.HandleFunc("/openwrt/{id}", auth(server.OpenWrtDetailHandler))
	router.HandleFunc("/openwrt/{id}/edit", auth(server.OpenWrtEditHandler))
	router.HandleFunc("/openwrt/{id}/test", auth(server.OpenWrtTestConnectionHandler))
	router.HandleFunc("/openwrt/{id}/reboot", auth(server.OpenWrtRebootHandler))
	router.HandleFunc("/openwrt/{id}/restart", auth(server.OpenWrtRestartServiceHandler))
	router.HandleFunc("/openwrt/{id}/network", auth(server.OpenWrtNetworkHandler))
	router.HandleFunc("/openwrt/{id}/dhcp", auth(server.OpenWrtDHCPHandler))
	router.HandleFunc("/openwrt/{id}/packages", auth(server.OpenWrtPackagesHandler))
	router.HandleFunc("/openwrt/{id}/firewall", auth(server.OpenWrtFirewallHandler))
	router.HandleFunc("/yeelight", auth(server.YeelightListView))
	router.HandleFunc("/yeelight/discover", auth(server.YeelightDiscoverHandler))
	router.HandleFunc("/yeelight/new", auth(server.YeelightNewHandler))
	router.HandleFunc("/yeelight/{id}", auth(server.YeelightDetailHandler))
	router.HandleFunc("/yeelight/{id}/edit", auth(server.YeelightEditHandler))
	router.HandleFunc("/yeelight/{id}/power", auth(server.YeelightPowerControlHandler))
	router.HandleFunc("/yeelight/{id}/brightness", auth(server.YeelightBrightnessControlHandler))
	router.HandleFunc("/yeelight/{id}/color-temp", auth(server.YeelightColorTempControlHandler))
	router.HandleFunc("/yeelight/{id}/color", auth(server.YeelightColorControlHandler))
	router.HandleFunc("/speedtest", auth(server.SpeedtestListView))
	router.HandleFunc("/speedtest/new", auth(server.SpeedtestNewHandler))
	router.HandleFunc("/speedtest/run", auth(server.SpeedtestRunHandler))
	router.HandleFunc("/speedtest/records", auth(server.SpeedtestRecordsHandler))
	router.HandleFunc("/speedtest/{id}/delete", auth(server.SpeedtestDeleteHandler))
	router.HandleFunc("/disks", auth(server.DisksView))
	router.HandleFunc("/disks/detail", auth(server.DiskDetailHandler))
	router.HandleFunc("/disks/test", auth(server.DiskTestHandler))
	router.HandleFunc("/web", auth(server.WebServerView))
	router.HandleFunc("/web/listeners/new", auth(server.WebListenerNewHandler))
	router.HandleFunc("/web/listeners/{id}", auth(server.WebListenerHandler))
	router.HandleFunc("/web/hosts/new", auth(server.WebHostNewHandler))
	router.HandleFunc("/web/hosts/{id}/edit", auth(server.WebHostEditHandler))
	router.HandleFunc("/web/hosts/{id}/routes", auth(server.WebHostRoutesHandler))
	router.HandleFunc("/web/hosts/{id}", auth(server.WebHostHandler))
	router.HandleFunc("/web/hosts/{id}/routes/new", auth(server.WebRouteNewHandler))
	router.HandleFunc("/web/routes/{id}", auth(server.WebRouteHandler))
	slog.Info("server starting", "addr", cfg.Listen)
	log.Fatal(http.ListenAndServe(cfg.Listen, loggingMiddleware(router)))
}
