package cron

import (
	"context"
	"errors"
	"fmt"
	"io"

	cronapi "github.com/lsongdev/ohmyserver/modules/cron/api"
	"github.com/lsongdev/ohmyserver/modules/cron/runner"
	"github.com/lsongdev/ohmyserver/modules/cron/store"
	zfsapi "github.com/lsongdev/ohmyserver/modules/zfsmgr/api"
)

const zfsTarget = "zfs"

// ZFSRunner lets a job run a pass over the ZFS module's retention policies.
// There is one target, and it ignores the job's command: what a pass does
// stays in the module, where its safety rules are.
type ZFSRunner struct{ Service *zfsapi.Service }

var _ runner.Runner = ZFSRunner{}

func (ZFSRunner) Kind() string { return zfsTarget }

func (ZFSRunner) Targets(context.Context) ([]runner.Target, error) {
	return []runner.Target{{ID: zfsTarget, Name: "ZFS snapshot schedules", Kind: zfsTarget, Internal: true}}, nil
}

// Run fails the job when any dataset failed; the pass has said which.
func (r ZFSRunner) Run(ctx context.Context, _ string, _ runner.Command, out io.Writer) (int, error) {
	err := r.Service.Apply(ctx, out)
	switch {
	case ctx.Err() != nil:
		return -1, ctx.Err()
	case err != nil:
		fmt.Fprintln(out, err)
		return 1, nil
	}
	return 0, nil
}

// ZFSJob is the job that runs those passes, as the ZFS module's Trigger. Cron
// is set once the scheduler exists, before the panel serves requests.
type ZFSJob struct{ Cron *cronapi.Service }

var _ zfsapi.Trigger = (*ZFSJob)(nil)

// Ensure creates the job if there is none. Passes go by what exists, so an
// hourly job also catches up on anything missed, at its next run.
func (j *ZFSJob) Ensure(ctx context.Context) error {
	_, err := j.ensure(ctx)
	return err
}

func (j *ZFSJob) ensure(ctx context.Context) (cronapi.JobView, error) {
	return j.Cron.EnsureJob(ctx, store.Job{
		Name: "ZFS snapshots", Schedule: "0 * * * *", Target: zfsTarget,
		Command: "# Takes due snapshots and removes old ones, per the schedules on ZFS datasets",
		Timeout: 3600, Enabled: true,
	})
}

// Status reports the job, or a zero status if it was deleted or moved to
// another target.
func (j *ZFSJob) Status(ctx context.Context) (zfsapi.TriggerStatus, error) {
	jobs, err := j.Cron.Jobs(ctx)
	if err != nil {
		return zfsapi.TriggerStatus{}, err
	}
	for _, job := range jobs {
		if job.Target != zfsTarget {
			continue
		}
		status := zfsapi.TriggerStatus{JobID: job.ID, Enabled: job.Enabled}
		if !job.NextRun.IsZero() {
			status.NextRun = &job.NextRun
		}
		if job.LastRun != nil {
			status.LastRun = &job.LastRun.StartedAt
			status.RunStatus = job.LastRun.Status
		}
		return status, nil
	}
	return zfsapi.TriggerStatus{}, nil
}

// Run starts a pass now, bringing a deleted job back first. A job turned off
// stays off, and a pass already going is enough.
func (j *ZFSJob) Run(ctx context.Context) error {
	job, err := j.ensure(ctx)
	if err != nil {
		return err
	}
	if !job.Enabled {
		return nil
	}
	if _, err := j.Cron.RunNow(ctx, job.ID); err != nil && !errors.Is(err, cronapi.ErrRunning) {
		return err
	}
	return nil
}
