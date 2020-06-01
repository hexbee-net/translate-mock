package main

import (
	"container/ring"
	"time"
)

//go:generate mockery -name=Throttler -case=snake -inpkg -testonly

type Throttler interface {
	PushCall()
	CheckThrottling() bool
}

type CallThrottler struct {
	Config *Config
	calls  *ring.Ring
}

func NewCallThrottler(config *Config) *CallThrottler {
	return &CallThrottler{
		Config: config,
		calls:  ring.New(config.ThrottlingThreshold),
	}
}

func (t *CallThrottler) PushCall() {
	t.calls.Value = time.Now()
	t.calls = t.calls.Next()
}

func (t *CallThrottler) CheckThrottling() bool {
	if t.Config.ThrottlingThreshold == 0 {
		return false
	}

	var (
		calls     int
		threshold = time.Now().Add(-1 * t.Config.ThrottlingPeriod)
	)
	for i := 0; i < t.calls.Len(); i++ {
		callTime := t.calls.Value.(time.Time)
		if callTime.After(threshold) {
			calls++
		}
	}

	if calls > t.Config.ThrottlingThreshold {
		return true
	}
	return false
}
