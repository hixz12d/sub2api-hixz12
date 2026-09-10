package service

import "context"

type MonitorPolicyControl interface {
	DisablePolicy(context.Context, int64, int64, int64) error
}
