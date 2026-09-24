package web

import (
	"context"
	"errors"
)

var ErrInvalidInput = errors.New("invalid cron job input")

// Service is the capability contract consumed by the cron HTTP adapter.
type Service interface {
	CronJobs(context.Context) ([]Job, error)
	CronJob(context.Context, int64) (Job, error)
	CreateCronJobAPI(context.Context, Input) (Job, error)
	UpdateCronJobAPI(context.Context, int64, Input) error
	DeleteCronJobAPI(context.Context, int64) error
	SetCronJobEnabled(context.Context, int64, bool) error
	RunCronJobNow(context.Context, int64) error
	CronJobExecutions(context.Context, int64, int) ([]Execution, error)
}
