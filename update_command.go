package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"time"

	"github.com/lsongdev/ohmyserver/buildinfo"
	"github.com/lsongdev/ohmyserver/updater"
)

func runUpdateCommand(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: ohmyserver update <check|apply>")
	}
	client := updater.NewClient()

	switch args[0] {
	case "check":
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		release, err := client.Latest(ctx)
		if err != nil {
			return err
		}
		asset, assetErr := updater.AssetName(runtime.GOOS, runtime.GOARCH)
		if assetErr != nil {
			return assetErr
		}
		_, hasBinary := release.Asset(asset)
		_, hasChecksums := release.Asset(updater.DefaultChecksumsAsset)
		fmt.Printf("current=%s commit=%s latest=%s binary=%t checksums=%t\n",
			buildinfo.Version, buildinfo.ShortCommit(), release.TagName, hasBinary, hasChecksums)
		return nil
	case "apply":
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		current, err := user.Current()
		if err != nil || current.Uid != "0" {
			return errors.New("update apply must run as root")
		}
		if runtime.GOOS != "linux" {
			return errors.New("automatic service update is supported only on Linux")
		}
		result, err := (&updater.Installer{
			Client: client,
			Target: updater.DefaultTarget,
		}).Apply(ctx)
		if err != nil {
			return err
		}
		fmt.Printf("installed=%s sha256=%s backup=%s\n",
			result.Release.TagName, result.SHA256, result.Backup)

		systemctl, err := exec.LookPath("systemctl")
		if err != nil {
			return fmt.Errorf("updated binary, but systemctl was not found: %w", err)
		}
		restart := exec.CommandContext(ctx, systemctl, "restart", "--no-block", updater.DefaultService)
		restart.Stdout = os.Stdout
		restart.Stderr = os.Stderr
		if err := restart.Run(); err != nil {
			return fmt.Errorf("updated binary, but failed to queue service restart: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unknown update command %q", args[0])
	}
}
