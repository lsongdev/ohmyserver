package uptime

import "context"

type Checker interface {
	CheckNow(context.Context, int64) (*Check, error)
}
