package gfx

import (
	"os"
	"strconv"
	"sync"
)

var (
	gfxParallelOnce sync.Once
	gfxParallelSem  chan struct{}
)

// MaxParallel returns configured GFX Chrome parallelism (default 3).
func MaxParallel() int {
	if v := os.Getenv("GFX_MAX_PARALLEL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 3
}

// AcquireParallel blocks until a GFX parallel slot is free. SEOShope does not use this.
func AcquireParallel() {
	gfxParallelOnce.Do(func() {
		gfxParallelSem = make(chan struct{}, MaxParallel())
	})
	gfxParallelSem <- struct{}{}
}

func ReleaseParallel() {
	if gfxParallelSem != nil {
		<-gfxParallelSem
	}
}
