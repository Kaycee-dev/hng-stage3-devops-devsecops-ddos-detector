package main

import (
	"context"
	"time"
)

func ScheduleUnban(ctx context.Context, duration time.Duration, callback func()) *time.Timer {
	timer := time.AfterFunc(duration, func() {
		select {
		case <-ctx.Done():
			return
		default:
			callback()
		}
	})
	go func() {
		<-ctx.Done()
		timer.Stop()
	}()
	return timer
}
