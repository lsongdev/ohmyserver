package pkgmgr

// The package intentionally has no dependency on OhMyServer, a database, an
// HTTP framework, or a privilege model. A control plane can inject separate
// query and mutation executors:
//
//	manager, err := pkgmgr.Detect(pkgmgr.Options{
//		MutatingExecutor: pkgmgr.Sudo(nil),
//	})
//	if err != nil {
//		// handle error
//	}
//	installed, err := manager.ListInstalled(ctx)
//	result, err := manager.Install(
//		ctx,
//		[]string{"curl"},
//		pkgmgr.ChangeOptions{AssumeYes: true},
//	)
//
// Package references are always passed as separate argv values and are never
// evaluated by a shell. OPKG upgrades without explicit package names first
// resolve list-upgradable and then invoke upgrade with that explicit set.
