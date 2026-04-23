package embedding

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

type BatchConfig struct {
	ChunkSize       int
	Concurrency     int
	MaxRetries      int
	RetryDelay      time.Duration
	RateLimitPerSec int
	Timeout         time.Duration
	ProgressFunc    func(progress BatchProgress)
}

func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		ChunkSize:       100,
		Concurrency:     4,
		MaxRetries:      3,
		RetryDelay:      1 * time.Second,
		RateLimitPerSec: 0,
		Timeout:         5 * time.Minute,
	}
}

type BatchProgress struct {
	Total     int
	Processed int
	Succeeded int
	Failed    int
	Elapsed   time.Duration
	ETA       time.Duration
	Rate      float64
	Current   int
	Message   string
	Done      bool
}

type BatchResult struct {
	Embeddings [][]float32
	Errors     []BatchError
	Total      int
	Succeeded  int
	Failed     int
	Duration   time.Duration
}

type BatchError struct {
	Index int
	Text  string
	Err   error
}

type BatchProcessor struct {
	manager *Manager
	cfg     BatchConfig
}

func (bp *BatchProcessor) Manager() *Manager   { return bp.manager }
func (bp *BatchProcessor) Config() BatchConfig { return bp.cfg }

func NewBatchProcessor(manager *Manager, cfg BatchConfig) *BatchProcessor {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 100
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 4
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 3
	}
	if cfg.RetryDelay <= 0 {
		cfg.RetryDelay = 1 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Minute
	}
	return &BatchProcessor{
		manager: manager,
		cfg:     cfg,
	}
}

func (bp *BatchProcessor) Process(ctx context.Context, texts []string) (*BatchResult, error) {
	if len(texts) == 0 {
		return &BatchResult{}, nil
	}

	ctx, cancel := context.WithTimeout(ctx, bp.cfg.Timeout)
	defer cancel()

	start := time.Now()
	total := len(texts)
	results := make([][]float32, total)
	var errors []BatchError
	var errorMu sync.Mutex

	var processed atomic.Int32
	var succeeded atomic.Int32
	var failed atomic.Int32

	sem := make(chan struct{}, bp.cfg.Concurrency)
	var wg sync.WaitGroup

	chunks := bp.splitChunks(texts)
	for chunkIdx, chunk := range chunks {
		for itemIdx, text := range chunk {
			globalIdx := chunkIdx*bp.cfg.ChunkSize + itemIdx

			if ctx.Err() != nil {
				for i := globalIdx; i < total; i++ {
					errorMu.Lock()
					errors = append(errors, BatchError{
						Index: i,
						Text:  texts[i],
						Err:   ctx.Err(),
					})
					errorMu.Unlock()
					failed.Add(1)
				}
				goto done
			}

			sem <- struct{}{}
			wg.Add(1)

			go func(idx int, t string) {
				defer wg.Done()
				defer func() { <-sem }()

				if bp.cfg.RateLimitPerSec > 0 {
					time.Sleep(time.Duration(float64(time.Second) / float64(bp.cfg.RateLimitPerSec)))
				}

				emb, err := bp.processWithRetry(ctx, t)
				if err != nil {
					failed.Add(1)
					errorMu.Lock()
					errors = append(errors, BatchError{
						Index: idx,
						Text:  t,
						Err:   err,
					})
					errorMu.Unlock()
				} else {
					results[idx] = emb
					succeeded.Add(1)
				}

				p := processed.Add(1)
				elapsed := time.Since(start)
				rate := float64(p) / elapsed.Seconds()
				remaining := total - int(p)
				eta := time.Duration(float64(remaining)/rate) * time.Second

				if bp.cfg.ProgressFunc != nil {
					bp.cfg.ProgressFunc(BatchProgress{
						Total:     total,
						Processed: int(p),
						Succeeded: int(succeeded.Load()),
						Failed:    int(failed.Load()),
						Elapsed:   elapsed,
						ETA:       eta,
						Rate:      rate,
						Current:   idx,
						Message:   fmt.Sprintf("processed %d/%d", p, total),
					})
				}
			}(globalIdx, text)
		}
	}

done:
	wg.Wait()

	duration := time.Since(start)

	if bp.cfg.ProgressFunc != nil {
		bp.cfg.ProgressFunc(BatchProgress{
			Total:     total,
			Processed: int(processed.Load()),
			Succeeded: int(succeeded.Load()),
			Failed:    int(failed.Load()),
			Elapsed:   duration,
			Done:      true,
			Message:   fmt.Sprintf("completed: %d succeeded, %d failed", succeeded.Load(), failed.Load()),
		})
	}

	return &BatchResult{
		Embeddings: results,
		Errors:     errors,
		Total:      total,
		Succeeded:  int(succeeded.Load()),
		Failed:     int(failed.Load()),
		Duration:   duration,
	}, nil
}

func (bp *BatchProcessor) processWithRetry(ctx context.Context, text string) ([]float32, error) {
	var lastErr error
	for attempt := 0; attempt <= bp.cfg.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := bp.cfg.RetryDelay * time.Duration(attempt)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		emb, err := bp.manager.Embed(ctx, text)
		if err == nil {
			return emb, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("after %d retries: %w", bp.cfg.MaxRetries, lastErr)
}

func (bp *BatchProcessor) splitChunks(texts []string) [][]string {
	var chunks [][]string
	for i := 0; i < len(texts); i += bp.cfg.ChunkSize {
		end := i + bp.cfg.ChunkSize
		if end > len(texts) {
			end = len(texts)
		}
		chunks = append(chunks, texts[i:end])
	}
	return chunks
}

func (bp *BatchProcessor) ProcessWithProvider(ctx context.Context, texts []string, providerIdx int) (*BatchResult, error) {
	if len(texts) == 0 {
		return &BatchResult{}, nil
	}

	providers := bp.manager.Providers()
	if providerIdx < 0 || providerIdx >= len(providers) {
		return nil, fmt.Errorf("invalid provider index: %d", providerIdx)
	}

	provider := providers[providerIdx]
	maxBatch := provider.Dimension()
	if maxBatch == 0 {
		maxBatch = 100
	}

	ctx, cancel := context.WithTimeout(ctx, bp.cfg.Timeout)
	defer cancel()

	start := time.Now()
	total := len(texts)
	results := make([][]float32, total)
	var errors []BatchError
	var errorMu sync.Mutex

	var processed atomic.Int32
	var succeeded atomic.Int32
	var failed atomic.Int32

	for i := 0; i < total; i += bp.cfg.ChunkSize {
		if ctx.Err() != nil {
			for j := i; j < total; j++ {
				errorMu.Lock()
				errors = append(errors, BatchError{
					Index: j,
					Text:  texts[j],
					Err:   ctx.Err(),
				})
				errorMu.Unlock()
				failed.Add(1)
			}
			break
		}

		end := i + bp.cfg.ChunkSize
		if end > total {
			end = total
		}
		chunk := texts[i:end]

		embeddings, err := provider.EmbedBatch(ctx, chunk)
		if err != nil {
			for j := i; j < end; j++ {
				errorMu.Lock()
				errors = append(errors, BatchError{
					Index: j,
					Text:  texts[j],
					Err:   err,
				})
				errorMu.Unlock()
				failed.Add(1)
			}
		} else {
			if err := validateBatchEmbeddings(provider, chunk, embeddings); err != nil {
				for j := i; j < end; j++ {
					errorMu.Lock()
					errors = append(errors, BatchError{
						Index: j,
						Text:  texts[j],
						Err:   err,
					})
					errorMu.Unlock()
					failed.Add(1)
				}
			} else {
				for j, emb := range embeddings {
					results[i+j] = emb
				}
				succeeded.Add(int32(len(embeddings)))
			}
		}

		p := processed.Add(int32(end - i))
		elapsed := time.Since(start)
		rate := float64(p) / elapsed.Seconds()
		remaining := total - int(p)
		eta := time.Duration(float64(remaining)/rate) * time.Second

		if bp.cfg.ProgressFunc != nil {
			bp.cfg.ProgressFunc(BatchProgress{
				Total:     total,
				Processed: int(p),
				Succeeded: int(succeeded.Load()),
				Failed:    int(failed.Load()),
				Elapsed:   elapsed,
				ETA:       eta,
				Rate:      rate,
				Current:   end - 1,
				Message:   fmt.Sprintf("processed %d/%d", p, total),
			})
		}
	}

	duration := time.Since(start)

	if bp.cfg.ProgressFunc != nil {
		bp.cfg.ProgressFunc(BatchProgress{
			Total:     total,
			Processed: int(processed.Load()),
			Succeeded: int(succeeded.Load()),
			Failed:    int(failed.Load()),
			Elapsed:   duration,
			Done:      true,
			Message:   fmt.Sprintf("completed: %d succeeded, %d failed", succeeded.Load(), failed.Load()),
		})
	}

	return &BatchResult{
		Embeddings: results,
		Errors:     errors,
		Total:      total,
		Succeeded:  int(succeeded.Load()),
		Failed:     int(failed.Load()),
		Duration:   duration,
	}, nil
}

func validateBatchEmbeddings(provider Provider, texts []string, embeddings [][]float32) error {
	if len(embeddings) != len(texts) {
		return fmt.Errorf(
			"embedding: provider %s returned %d embeddings for %d texts",
			provider.Name(),
			len(embeddings),
			len(texts),
		)
	}

	for i, embedding := range embeddings {
		if embedding == nil {
			return fmt.Errorf(
				"embedding: provider %s returned nil embedding at index %d",
				provider.Name(),
				i,
			)
		}
	}

	return nil
}

func (bp *BatchProcessor) EstimateCost(texts []string, providerIdx int) (tokens int, cost float64, err error) {
	providers := bp.manager.Providers()
	if providerIdx < 0 || providerIdx >= len(providers) {
		return 0, 0, fmt.Errorf("invalid provider index: %d", providerIdx)
	}

	provider := providers[providerIdx]
	_ = provider

	totalTokens := 0
	for _, text := range texts {
		tokens := len(text) / 4
		if tokens < 1 {
			tokens = 1
		}
		totalTokens += tokens
	}

	return totalTokens, 0, nil
}
