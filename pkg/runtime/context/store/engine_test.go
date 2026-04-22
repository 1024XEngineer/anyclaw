package store

import (
	"context"
	"testing"
)

func TestSearchWithZeroTopKReturnsAllMatches(t *testing.T) {
	engine := NewInMemoryContextEngine()
	ctx := context.Background()

	if err := engine.AddDocument(ctx, Document{ID: "doc-1", Content: "go runtime"}); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}
	if err := engine.AddDocument(ctx, Document{ID: "doc-2", Content: "go context"}); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	results, err := engine.Search(ctx, "go", SearchOptions{Threshold: 0.1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected all matches when TopK is zero, got %d", len(results))
	}
}

func TestSearchReturnsDefensiveCopies(t *testing.T) {
	engine := NewInMemoryContextEngine()
	ctx := context.Background()

	original := Document{
		ID:       "doc-1",
		Content:  "go runtime store",
		Metadata: map[string]any{"scope": "runtime"},
		Vector:   []float64{1, 2, 3},
	}
	if err := engine.AddDocument(ctx, original); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	results, err := engine.Search(ctx, "runtime", SearchOptions{Threshold: 0.1, TopK: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected one result, got %d", len(results))
	}

	results[0].Document.Content = "mutated"
	results[0].Document.Metadata["scope"] = "changed"
	results[0].Document.Vector[0] = 99

	stored, err := engine.GetDocument(ctx, "doc-1")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if stored.Content != "go runtime store" {
		t.Fatalf("expected stored content unchanged, got %q", stored.Content)
	}
	if stored.Metadata["scope"] != "runtime" {
		t.Fatalf("expected stored metadata unchanged, got %#v", stored.Metadata["scope"])
	}
	if stored.Vector[0] != 1 {
		t.Fatalf("expected stored vector unchanged, got %v", stored.Vector)
	}
}

func TestGetDocumentReturnsDefensiveCopy(t *testing.T) {
	engine := NewInMemoryContextEngine()
	ctx := context.Background()

	if err := engine.AddDocument(ctx, Document{
		ID:       "doc-1",
		Content:  "go runtime",
		Metadata: map[string]any{"scope": "runtime"},
		Vector:   []float64{1, 2, 3},
	}); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	doc, err := engine.GetDocument(ctx, "doc-1")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}

	doc.Content = "mutated"
	doc.Metadata["scope"] = "changed"
	doc.Vector[1] = 88

	stored, err := engine.GetDocument(ctx, "doc-1")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if stored.Content != "go runtime" {
		t.Fatalf("expected stored content unchanged, got %q", stored.Content)
	}
	if stored.Metadata["scope"] != "runtime" {
		t.Fatalf("expected stored metadata unchanged, got %#v", stored.Metadata["scope"])
	}
	if stored.Vector[1] != 2 {
		t.Fatalf("expected stored vector unchanged, got %v", stored.Vector)
	}
}

func TestAddDocumentStoresDefensiveCopy(t *testing.T) {
	engine := NewInMemoryContextEngine()
	ctx := context.Background()

	doc := Document{
		ID:       "doc-1",
		Content:  "go runtime",
		Metadata: map[string]any{"scope": "runtime"},
		Vector:   []float64{1, 2, 3},
	}
	if err := engine.AddDocument(ctx, doc); err != nil {
		t.Fatalf("AddDocument: %v", err)
	}

	doc.Content = "mutated"
	doc.Metadata["scope"] = "changed"
	doc.Vector[2] = 77

	stored, err := engine.GetDocument(ctx, "doc-1")
	if err != nil {
		t.Fatalf("GetDocument: %v", err)
	}
	if stored.Content != "go runtime" {
		t.Fatalf("expected stored content unchanged, got %q", stored.Content)
	}
	if stored.Metadata["scope"] != "runtime" {
		t.Fatalf("expected stored metadata unchanged, got %#v", stored.Metadata["scope"])
	}
	if stored.Vector[2] != 3 {
		t.Fatalf("expected stored vector unchanged, got %v", stored.Vector)
	}
}
