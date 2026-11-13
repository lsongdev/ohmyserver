// Command xray-go serves the Xray module on its own.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/lsongdev/ohmyserver/internal/shell"
	"github.com/lsongdev/ohmyserver/modules/xray/web"
)

func main() {
	dir, err := os.UserConfigDir()
	if err == nil {
		dir = filepath.Join(dir, "xray-go")
	} else {
		dir = "./xray-data"
	}
	data := flag.String("d", dir, "directory for saved Xray configuration")
	listen := flag.String("listen", "127.0.0.1:8090", "management UI and API address")
	flag.Parse()
	if err := run(*listen, *data, os.Getenv("XRAY_GO_PASSWORD")); err != nil {
		log.Fatal(err)
	}
}

func run(listen, data, password string) error {
	if password == "" && !shell.IsLoopback(listen) {
		return fmt.Errorf("set XRAY_GO_PASSWORD to listen outside loopback")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	module, err := web.New(data)
	if err != nil {
		return err
	}
	defer module.Close()
	log.Printf("Xray panel listening on http://%s", listen)
	return shell.ListenAndServe(ctx, listen, module.Standalone(password))
}
