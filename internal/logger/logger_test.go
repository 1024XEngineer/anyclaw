package logger

import (
	"sync"
	"testing"

	"go.uber.org/zap"
)

func TestConcurrentInit(t *testing.T) {
	// Reset for test
	log = nil
	sugar = nil
	once = sync.Once{}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = Init("info", false)
		}()
	}

	wg.Wait()

	if log == nil {
		t.Fatal("logger not initialized")
	}
	if sugar == nil {
		t.Fatal("sugar logger not initialized")
	}
}

func TestConcurrentAccess(t *testing.T) {
	_ = Init("debug", true)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			L().Info("test", zap.Int("i", i))
		}(i)
		go func(i int) {
			defer wg.Done()
			S().Infof("test %d", i)
		}(i)
		go func(i int) {
			defer wg.Done()
			Info("test", zap.Int("i", i))
		}(i)
	}

	wg.Wait()
	_ = Sync()
}
