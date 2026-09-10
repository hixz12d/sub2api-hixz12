package service

import "context"

type MonitorPolicyManagement interface {
	ListPolicies(context.Context, int, int) ([]ChannelMonitorGroupPolicy, error)
	SavePolicy(context.Context, ChannelMonitorGroupPolicy, int64, int64) (*ChannelMonitorGroupPolicy, error)
	DeletePolicy(context.Context, int64, int64, int64) error
}
