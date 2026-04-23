package index

import (
	"context"
	"fmt"
	"time"

	"github.com/1024XEngineer/anyclaw/pkg/vec"
)

func (im *IndexManager) Index(ctx context.Context, indexName string, items []IndexItem, progress ProgressFunc) (*IndexResult, error) {
	current, err := im.getIndex(indexName)
	if err != nil {
		return nil, err
	}
	if current.Status != StatusReady {
		return nil, fmt.Errorf("index %q is not ready (status: %s)", indexName, current.Status)
	}

	updating := cloneIndexInfo(current)
	updating.Status = StatusUpdating
	updating.Error = ""
	updating.UpdatedAt = time.Now()
	if _, err := im.persistIndex(ctx, updating); err != nil {
		return nil, err
	}

	backend, err := im.openBackend(ctx, updating)
	if err != nil {
		im.markIndexError(ctx, indexName, err)
		return nil, fmt.Errorf("open vector backend: %w", err)
	}
	defer backend.Close()

	start := time.Now()
	result := &IndexResult{
		IndexName: indexName,
		Total:     len(items),
		StartedAt: start,
	}

	const batchSize = 100
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		batch := items[i:end]
		vecItems := make([]vec.VecItem, 0, len(batch))

		for _, item := range batch {
			if item.Vector != nil {
				if len(item.Vector) != updating.Dimensions {
					result.Failed++
					continue
				}
				vecItems = append(vecItems, vec.VecItem{
					ID:       item.ID,
					Vector:   append([]float32(nil), item.Vector...),
					Metadata: cloneStringMap(item.Metadata),
					Aux:      cloneStringMap(item.Aux),
				})
				continue
			}

			if im.embedder == nil || item.Text == "" {
				result.Failed++
				continue
			}

			embedding, err := im.embedder.Embed(ctx, item.Text)
			if err != nil {
				result.Failed++
				if progress != nil {
					progress(Progress{
						Total:     len(items),
						Processed: i + 1,
						Failed:    result.Failed,
						Elapsed:   time.Since(start),
						CurrentID: item.ID,
						Message:   fmt.Sprintf("embed failed: %v", err),
					})
				}
				continue
			}

			vecItems = append(vecItems, vec.VecItem{
				ID:       item.ID,
				Vector:   embedding,
				Metadata: cloneStringMap(item.Metadata),
				Aux:      cloneStringMap(item.Aux),
			})
		}

		if len(vecItems) > 0 {
			if err := backend.InsertBatch(ctx, vecItems); err != nil {
				result.Failed += len(vecItems)
				if progress != nil {
					progress(Progress{
						Total:     len(items),
						Processed: end,
						Failed:    result.Failed,
						Elapsed:   time.Since(start),
						CurrentID: batch[len(batch)-1].ID,
						Message:   fmt.Sprintf("insert batch failed: %v", err),
					})
				}
			} else {
				result.Indexed += len(vecItems)
			}
		}

		if progress != nil {
			elapsed := time.Since(start)
			progress(Progress{
				Total:     len(items),
				Processed: end,
				Failed:    result.Failed,
				Elapsed:   elapsed,
				ETA:       estimateETA(end, len(items), elapsed),
				CurrentID: batch[len(batch)-1].ID,
				Message:   fmt.Sprintf("indexed %d/%d", end, len(items)),
			})
		}
	}

	count, err := backend.Count(ctx)
	if err != nil {
		count = current.VectorCount
	}

	ready := cloneIndexInfo(current)
	ready.Status = StatusReady
	ready.Error = ""
	ready.UpdatedAt = time.Now()
	ready.VectorCount = count
	if _, err := im.persistIndex(ctx, ready); err != nil {
		return nil, err
	}

	result.CompletedAt = time.Now()
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	if progress != nil {
		progress(Progress{
			Total:     len(items),
			Processed: len(items),
			Failed:    result.Failed,
			Elapsed:   result.Duration,
			Done:      true,
			Message:   fmt.Sprintf("completed: %d indexed, %d failed", result.Indexed, result.Failed),
		})
	}

	return result, nil
}

func (im *IndexManager) RemoveVectors(ctx context.Context, indexName string, ids []int64) (int, error) {
	current, err := im.getIndex(indexName)
	if err != nil {
		return 0, err
	}

	backend, err := im.openBackend(ctx, current)
	if err != nil {
		return 0, fmt.Errorf("open vector backend: %w", err)
	}
	defer backend.Close()

	removed := 0
	for _, id := range ids {
		if err := backend.Delete(ctx, id); err == nil {
			removed++
		}
	}

	count, err := backend.Count(ctx)
	if err != nil {
		count = current.VectorCount
	}

	updated := cloneIndexInfo(current)
	updated.UpdatedAt = time.Now()
	updated.VectorCount = count
	if _, err := im.persistIndex(ctx, updated); err != nil {
		return 0, err
	}

	return removed, nil
}

func (im *IndexManager) Rebuild(ctx context.Context, indexName string, progress ProgressFunc) (*IndexResult, error) {
	current, err := im.getIndex(indexName)
	if err != nil {
		return nil, err
	}

	rebuilding := cloneIndexInfo(current)
	rebuilding.Status = StatusRebuilding
	rebuilding.Error = ""
	rebuilding.UpdatedAt = time.Now()
	if _, err := im.persistIndex(ctx, rebuilding); err != nil {
		return nil, err
	}

	start := time.Now()
	backend, err := im.newBackend(rebuilding)
	if err != nil {
		im.markIndexError(ctx, indexName, err)
		return nil, err
	}
	defer backend.Close()

	if err := backend.Drop(ctx); err != nil {
		im.markIndexError(ctx, indexName, err)
		return nil, fmt.Errorf("drop vector collection: %w", err)
	}
	if err := backend.Init(ctx); err != nil {
		im.markIndexError(ctx, indexName, err)
		return nil, fmt.Errorf("recreate vector collection: %w", err)
	}
	if err := backend.EnsurePayloadIndexes(ctx); err != nil {
		im.markIndexError(ctx, indexName, err)
		return nil, fmt.Errorf("create payload indexes: %w", err)
	}

	ready := cloneIndexInfo(current)
	ready.Status = StatusReady
	ready.Error = ""
	ready.UpdatedAt = time.Now()
	ready.VectorCount = 0
	if _, err := im.persistIndex(ctx, ready); err != nil {
		return nil, err
	}

	result := &IndexResult{
		IndexName:   indexName,
		StartedAt:   start,
		CompletedAt: time.Now(),
	}
	result.Duration = result.CompletedAt.Sub(result.StartedAt)

	if progress != nil {
		progress(Progress{
			Total:   0,
			Done:    true,
			Elapsed: result.Duration,
			Message: "index rebuilt (empty, re-index needed)",
		})
	}

	return result, nil
}

func (im *IndexManager) Search(ctx context.Context, indexName string, queryVector []float32, limit int) ([]vec.VecSearchResult, error) {
	return im.SearchWithFilter(ctx, indexName, queryVector, limit, nil)
}

func (im *IndexManager) SearchWithFilter(ctx context.Context, indexName string, queryVector []float32, limit int, metadataFilter map[string]string) ([]vec.VecSearchResult, error) {
	current, err := im.getIndex(indexName)
	if err != nil {
		return nil, err
	}

	backend, err := im.openBackend(ctx, current)
	if err != nil {
		return nil, fmt.Errorf("open vector backend: %w", err)
	}
	defer backend.Close()

	return backend.SearchWithFilter(ctx, queryVector, limit, metadataFilter)
}

func (im *IndexManager) SearchByText(ctx context.Context, indexName string, queryText string, limit int) ([]vec.VecSearchResult, error) {
	return im.SearchByTextWithFilter(ctx, indexName, queryText, limit, nil)
}

func (im *IndexManager) SearchByTextWithFilter(ctx context.Context, indexName string, queryText string, limit int, metadataFilter map[string]string) ([]vec.VecSearchResult, error) {
	if im.embedder == nil {
		return nil, fmt.Errorf("no embedder configured")
	}

	queryVector, err := im.embedder.Embed(ctx, queryText)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	return im.SearchWithFilter(ctx, indexName, queryVector, limit, metadataFilter)
}

func estimateETA(processed, total int, elapsed time.Duration) time.Duration {
	if processed <= 0 || total <= processed || elapsed <= 0 {
		return 0
	}

	rate := float64(processed) / elapsed.Seconds()
	if rate <= 0 {
		return 0
	}

	remaining := total - processed
	return time.Duration(float64(remaining)/rate) * time.Second
}
