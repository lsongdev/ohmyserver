# Each UI (the shell's and every module's) declares its browser
# dependencies in its own package.json; Go embeds them from its
# node_modules, so `make deps` (npm ci in each UI) runs before a build.

UIS := internal/shell/ui/static $(patsubst %/package.json,%,$(wildcard modules/*/web/ui/package.json))

.PHONY: build test deps clean

build: deps
	go build -o ohmyserver ./cmd/ohmyserver

test: deps
	go vet ./...
	go test ./...

deps: $(addsuffix /node_modules,$(UIS))

%/node_modules: %/package.json %/package-lock.json
	npm ci --prefix $* --no-audit --no-fund
	@touch $@

clean:
	rm -rf ohmyserver $(addsuffix /node_modules,$(UIS))
