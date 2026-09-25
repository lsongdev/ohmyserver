package host

import (
	"context"
	"errors"
	"fmt"
	"log"
)

// Lifecycle steps are listed here so schema ownership and process startup
// order remain visible at the host composition boundary.
func (s *Server) initializeModuleSchemas() error {
	steps := []struct {
		name string
		run  func() error
	}{
		{"clash", s.initClashSchema},
		{"projects", func() error { return s.projectRepository().Initialize(context.Background()) }},
		{"forward", s.initTCPListenerSchema},
		{"esxi", s.initESXiServerSchema},
		{"acme", s.initACMESchema},
		{"ssh", s.initSSHServerSchema},
		{"dhcp", s.initDHCPSchema},
		{"dns", s.initDNSSchema},
		{"docker apps", s.initAppsSchema},
		{"idrac", s.initIDRACSchema},
		{"ipmi", s.initIPMISchema},
		{"monitor", s.initMonitorSchema},
		{"ntp", s.initNTPSchema},
		{"openwrt", s.initOpenWrtSchema},
		{"pve", s.initPVESchema},
		{"service preferences", s.initServicesSchema},
		{"settings", s.initSettingsSchema},
		{"speedtest", s.initSpeedtestSchema},
		{"uptime", s.initUptimeSchema},
		{"webserver", s.initWebServerSchema},
		{"widgets", s.initWidgetSchema},
		{"yeelight", s.initYeelightSchema},
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			return fmt.Errorf("initialize %s module: %w", step.name, err)
		}
	}
	return nil
}

func (s *Server) runLegacyMigrations() error {
	// These are one-time bridges for schemas from before the module migration.
	for _, query := range []string{`ALTER TABLE llm_models ADD COLUMN mapped_name TEXT`} {
		_, _ = s.db.Exec(query)
	}
	if err := s.projectRepository().Migrate(context.Background()); err != nil {
		return err
	}
	return s.migrateLegacyWebWorkerScripts()
}

func (s *Server) startServices() {
	steps := []struct {
		name string
		run  func() error
	}{
		{"logger", s.InitSlog},
		{"dhcp", func() error {
			err := s.localDHCPModule().Start(s.dhcpLifecycleContext())
			s.setServiceError("dhcp", err)
			return err
		}},
		{"dns", s.startDNSService},
		{"docker app catalog", s.syncDockerAppCatalog},
		{"monitor", s.startMonitorCollection},
		{"ntp", s.startNTPService},
		{"projects", func() error { return s.projectRepository().RecoverInterruptedDeployments(context.Background()) }},
		{"forward", s.startForwardService},
		{"webserver", s.startWebServer},
	}
	for _, step := range steps {
		if err := step.run(); err != nil {
			log.Printf("Warning: initialize %s service: %v", step.name, err)
		}
	}
}

func (s *Server) stopServices() error {
	closeErr := closeSteps(
		func() error { return wrapCloseError("webserver", s.stopWebServer()) },
		func() error { return wrapCloseError("forward", s.stopForwardService()) },
		func() error { return wrapCloseError("ntp", s.stopNTPService()) },
		func() error { return wrapCloseError("dns", s.stopDNSService()) },
		func() error {
			if s.dhcpModule == nil {
				return nil
			}
			return wrapCloseError("dhcp", s.dhcpModule.Close())
		},
	)
	if closeErr != nil {
		log.Printf("Close hook error: %v", closeErr)
	}
	return closeErr
}

func wrapCloseError(name string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("close %s service: %w", name, err)
}

func closeSteps(steps ...func() error) error {
	var closeErr error
	for _, step := range steps {
		closeErr = errors.Join(closeErr, step())
	}
	return closeErr
}
