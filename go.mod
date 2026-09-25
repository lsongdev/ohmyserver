module github.com/lsongdev/ohmyserver

go 1.26.1

require (
	github.com/docker/docker v25.0.4+incompatible
	github.com/lsongdev/dhcp-go v0.0.0-20260728081411-bb497a8e114c
	github.com/lsongdev/dns-go v0.0.0-20260526003909-ad575eb83682
	github.com/lsongdev/ntp-go v0.0.0-20260728080956-58638dac0cb4
	github.com/lsongdev/yabber v0.0.0-20260729032210-7ba542731aae
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.37.1
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20250102033503-faa5f7b0171c // indirect
	github.com/bougou/go-ipmi v0.8.2 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cloudflare/cloudflare-go v0.118.0 // indirect
	github.com/creack/pty v1.1.24 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.30.0 // indirect
	github.com/lsongdev/acme-go v0.0.0-20260403095516-04f0b6832575 // indirect
	github.com/lsongdev/clashapi-go v0.0.0-20260506085541-7d037170d5b5 // indirect
	github.com/lsongdev/idrac-go v0.0.0-20260406044348-bf8a10c471b3 // indirect
	github.com/lsongdev/openwrt-go v0.0.0-20260411153216-6c064a0962c3 // indirect
	github.com/lsongdev/speedtest-go v0.0.0-20260423054920-93af072d218e // indirect
	github.com/lsongdev/yeelight-go v0.0.0-20260429060907-76de1d03edb6 // indirect
	github.com/namedotcom/go v0.0.0-20180403034216-08470befbe04 // indirect
	github.com/nrdcg/dnspod-go v0.4.0 // indirect
	github.com/shirou/gopsutil/v3 v3.24.5 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/vmware/govmomi v0.53.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.46.0 // indirect
	go.opentelemetry.io/proto/otlp v1.11.0 // indirect
	golang.org/x/crypto v0.55.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260819154853-08b0e4226688 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260819154853-08b0e4226688 // indirect
	google.golang.org/grpc v1.83.1 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)

require (
	github.com/Microsoft/go-winio v0.4.14 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/dlclark/regexp2 v1.11.4 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/dop251/goja v0.0.0-20260311135729-065cd970411c // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/fatih/color v1.15.0 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-sourcemap/sourcemap v2.1.3+incompatible // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/pprof v0.0.0-20250317173921-a4b03ec1a45e // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/kr/pretty v0.3.1 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lsongdev/ohmyserver/modules/acme v0.0.0
	github.com/lsongdev/ohmyserver/modules/clash v0.0.0
	github.com/lsongdev/ohmyserver/modules/cron v0.0.0
	github.com/lsongdev/ohmyserver/modules/dashboard v0.0.0
	github.com/lsongdev/ohmyserver/modules/dhcp v0.0.0
	github.com/lsongdev/ohmyserver/modules/disks v0.0.0
	github.com/lsongdev/ohmyserver/modules/dns v0.0.0
	github.com/lsongdev/ohmyserver/modules/docker v0.0.0
	github.com/lsongdev/ohmyserver/modules/esxi v0.0.0
	github.com/lsongdev/ohmyserver/modules/files v0.0.0
	github.com/lsongdev/ohmyserver/modules/forward v0.0.0
	github.com/lsongdev/ohmyserver/modules/idrac v0.0.0
	github.com/lsongdev/ohmyserver/modules/ipmi v0.0.0
	github.com/lsongdev/ohmyserver/modules/monitor v0.0.0
	github.com/lsongdev/ohmyserver/modules/ntp v0.0.0
	github.com/lsongdev/ohmyserver/modules/openwrt v0.0.0
	github.com/lsongdev/ohmyserver/modules/packages v0.0.0
	github.com/lsongdev/ohmyserver/modules/projects v0.0.0
	github.com/lsongdev/ohmyserver/modules/pve v0.0.0
	github.com/lsongdev/ohmyserver/modules/settings v0.0.0
	github.com/lsongdev/ohmyserver/modules/speedtest v0.0.0
	github.com/lsongdev/ohmyserver/modules/ssh v0.0.0
	github.com/lsongdev/ohmyserver/modules/system v0.0.0
	github.com/lsongdev/ohmyserver/modules/terminal v0.0.0
	github.com/lsongdev/ohmyserver/modules/uptime v0.0.0
	github.com/lsongdev/ohmyserver/modules/webserver v0.0.0
	github.com/lsongdev/ohmyserver/modules/widgets v0.0.0
	github.com/lsongdev/ohmyserver/modules/yeelight v0.0.0
	github.com/lsongdev/ohmyserver/pkg/terminal v0.0.0
	github.com/lsongdev/ssdp-go v0.0.0-20260411070512-aece6ff5803f // indirect
	github.com/lsongdev/workers-go v0.0.0-20260406114953-755eda039598 // indirect
	github.com/lufia/plan9stats v0.0.0-20211012122336-39d0f177ccd0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.16 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/olekukonko/errors v1.1.0 // indirect
	github.com/olekukonko/ll v0.0.9 // indirect
	github.com/olekukonko/tablewriter v1.0.9 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/power-devops/perfstat v0.0.0-20210106213030-5aafc221ea8c // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/shoenig/go-m1cpu v0.1.6 // indirect
	github.com/tklauser/go-sysconf v0.3.12 // indirect
	github.com/tklauser/numcpus v0.6.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.71.0 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/term v0.45.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/time v0.16.0 // indirect
	modernc.org/libc v1.75.7 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

replace github.com/lsongdev/ohmyserver/modules/docker => ./modules/docker

replace github.com/lsongdev/ohmyserver/pkg/moduleweb => ./pkg/moduleweb

replace github.com/lsongdev/ohmyserver/modules/dashboard => ./modules/dashboard

replace github.com/lsongdev/ohmyserver/modules/system => ./modules/system

replace github.com/lsongdev/ohmyserver/modules/settings => ./modules/settings

replace github.com/lsongdev/ohmyserver/modules/files => ./modules/files

replace github.com/lsongdev/ohmyserver/modules/clash => ./modules/clash

replace github.com/lsongdev/ohmyserver/modules/uptime => ./modules/uptime

replace github.com/lsongdev/ohmyserver/modules/disks => ./modules/disks

replace github.com/lsongdev/ohmyserver/modules/esxi => ./modules/esxi

replace github.com/lsongdev/ohmyserver/modules/idrac => ./modules/idrac

replace github.com/lsongdev/ohmyserver/modules/ipmi => ./modules/ipmi

replace github.com/lsongdev/ohmyserver/modules/packages => ./modules/packages

replace github.com/lsongdev/ohmyserver/modules/projects => ./modules/projects

replace github.com/lsongdev/ohmyserver/modules/pve => ./modules/pve

replace github.com/lsongdev/ohmyserver/modules/speedtest => ./modules/speedtest

replace github.com/lsongdev/ohmyserver/modules/ssh => ./modules/ssh

replace github.com/lsongdev/ohmyserver/modules/webserver => ./modules/webserver

replace github.com/lsongdev/ohmyserver/modules/yeelight => ./modules/yeelight

replace github.com/lsongdev/ohmyserver/modules/acme => ./modules/acme

replace github.com/lsongdev/ohmyserver/modules/dhcp => ./modules/dhcp

replace github.com/lsongdev/ohmyserver/modules/dns => ./modules/dns

replace github.com/lsongdev/ohmyserver/modules/forward => ./modules/forward

replace github.com/lsongdev/ohmyserver/modules/monitor => ./modules/monitor

replace github.com/lsongdev/ohmyserver/modules/ntp => ./modules/ntp

replace github.com/lsongdev/ohmyserver/modules/openwrt => ./modules/openwrt

replace github.com/lsongdev/ohmyserver/modules/widgets => ./modules/widgets

replace github.com/lsongdev/ohmyserver/pkg/terminal => ./pkg/terminal

replace github.com/lsongdev/ohmyserver/modules/cron => ./modules/cron

replace github.com/lsongdev/ohmyserver/modules/terminal => ./modules/terminal
