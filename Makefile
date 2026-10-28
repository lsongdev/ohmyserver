# Each UI (the shell's and every module's) declares its browser
# dependencies in its own package.json; Go embeds them from its
# node_modules, so `make deps` (npm ci in each UI) runs before a build.
#
# The modules that are projects of their own (acme-go, dns-go, …) are
# built from sibling checkouts (see go.work), as CI does too.

UIS := internal/shell/ui/static $(patsubst %/package.json,%,$(wildcard modules/*/web/ui/package.json))

# Version defaults to the git description; CI passes a date such as 2026.10.28.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
GOFLAGS := -trimpath
DIST    := dist

# GOOS/GOARCH[/GOARM]. The SQLite driver is pure Go but has no MIPS port,
# and the file manager needs a Unix, so there are no MIPS or Windows builds.
PLATFORMS := \
	linux/amd64 \
	linux/arm64 \
	linux/arm/7 \
	linux/riscv64 \
	darwin/amd64 \
	darwin/arm64 \
	freebsd/amd64

.PHONY: build test deps dist clean $(PLATFORMS)

build: deps
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o ohmyserver ./cmd/ohmyserver

test: deps
	go vet ./...
	go test ./...

deps: $(addsuffix /node_modules,$(UIS))

%/node_modules: %/package.json %/package-lock.json
	npm ci --prefix $* --no-audit --no-fund
	@touch $@

# Release files: a binary per platform and the systemd unit, all covered
# by SHA256SUMS.
dist: $(PLATFORMS)
	cp contrib/systemd/ohmyserver.service $(DIST)/ohmyserver.service
	cd $(DIST) && shasum -a 256 ohmyserver_* ohmyserver.service > SHA256SUMS

# Expanded per recipe from the target name, so parallel builds are safe.
os      = $(word 1,$(subst /, ,$@))
arch    = $(word 2,$(subst /, ,$@))
variant = $(word 3,$(subst /, ,$@))
binary  = ohmyserver_$(os)_$(arch)$(if $(variant),_v$(variant))

$(PLATFORMS): deps
	@mkdir -p $(DIST)
	CGO_ENABLED=0 GOOS=$(os) GOARCH=$(arch) $(if $(variant),GOARM=$(variant)) \
		go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(DIST)/$(binary) ./cmd/ohmyserver

clean:
	rm -rf ohmyserver $(DIST) $(addsuffix /node_modules,$(UIS))
