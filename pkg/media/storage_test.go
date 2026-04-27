package media

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLocalStorage_PutAndGet(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()
	data := []byte("test media content")

	obj, err := store.Put(ctx, "test/file.txt", data, StoragePutOptions{
		MimeType: "text/plain",
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	if obj.Key != "test/file.txt" {
		t.Errorf("expected key test/file.txt, got %s", obj.Key)
	}
	if obj.Size != int64(len(data)) {
		t.Errorf("expected size %d, got %d", len(data), obj.Size)
	}
	if obj.MimeType != "text/plain" {
		t.Errorf("expected mime text/plain, got %s", obj.MimeType)
	}

	got, err := store.Get(ctx, "test/file.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.Key != "test/file.txt" {
		t.Errorf("expected key test/file.txt, got %s", got.Key)
	}
	if got.Size != int64(len(data)) {
		t.Errorf("expected size %d, got %d", len(data), got.Size)
	}
}

func TestLocalStorage_Delete(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()
	_, err := store.Put(ctx, "to-delete.txt", []byte("delete me"), StoragePutOptions{})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	err = store.Delete(ctx, "to-delete.txt")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	exists, err := store.Exists(ctx, "to-delete.txt")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists {
		t.Error("expected file to be deleted")
	}
}

func TestLocalStorage_DeleteNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()
	err := store.Delete(ctx, "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for deleting nonexistent file")
	}
}

func TestLocalStorage_Exists(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()

	exists, err := store.Exists(ctx, "missing.txt")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists {
		t.Error("expected false for missing file")
	}

	_, err = store.Put(ctx, "present.txt", []byte("hello"), StoragePutOptions{})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	exists, err = store.Exists(ctx, "present.txt")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Error("expected true for existing file")
	}
}

func TestLocalStorage_List(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()

	_, _ = store.Put(ctx, "img1.jpg", []byte("img1"), StoragePutOptions{})
	_, _ = store.Put(ctx, "img2.jpg", []byte("img2"), StoragePutOptions{})
	_, _ = store.Put(ctx, "doc.pdf", []byte("doc"), StoragePutOptions{})

	objects, err := store.List(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(objects) != 3 {
		t.Errorf("expected 3 objects, got %d", len(objects))
	}
}

func TestLocalStorage_URL(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://example.com/media",
	})

	ctx := context.Background()

	objURL, err := store.URL(ctx, "photo.jpg", 0)
	if err != nil {
		t.Fatalf("url: %v", err)
	}

	expected := "https://example.com/media/photo.jpg"
	if objURL != expected {
		t.Errorf("expected URL %s, got %s", expected, objURL)
	}
}

func TestLocalStorage_URL_NoBaseURL(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()

	objURL, err := store.URL(ctx, "photo.jpg", 0)
	if err != nil {
		t.Fatalf("url: %v", err)
	}

	expected := "file://" + filepath.Join(dir, "photo.jpg")
	if objURL != expected {
		t.Errorf("expected URL %s, got %s", expected, objURL)
	}
}

func TestLocalStorage_Type(t *testing.T) {
	store := NewLocalStorage(LocalStorageConfig{})
	if store.Type() != StorageLocal {
		t.Errorf("expected StorageLocal, got %s", store.Type())
	}
}

func TestLocalStorage_PutWithMetadata(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()
	_, err := store.Put(ctx, "meta.txt", []byte("data"), StoragePutOptions{
		MimeType: "text/plain",
		Metadata: map[string]string{
			"author": "test",
			"title":  "example",
		},
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	obj, err := store.Get(ctx, "meta.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if !strings.HasPrefix(obj.MimeType, "text/plain") {
		t.Errorf("expected mime starting with text/plain, got %s", obj.MimeType)
	}

	if len(obj.Metadata) == 0 {
		t.Error("expected metadata to be populated")
	}
}

func TestS3Storage_Type(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:   "us-east-1",
		Bucket:   "test-bucket",
		Endpoint: "s3.us-east-1.amazonaws.com",
	})

	if store.Type() != StorageS3 {
		t.Errorf("expected StorageS3, got %s", store.Type())
	}
}

func TestS3Storage_HostURL_VirtualHosted(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:   "us-east-1",
		Bucket:   "my-bucket",
		Endpoint: "s3.us-east-1.amazonaws.com",
	})

	host := store.hostURL()
	if host.Host != "my-bucket.s3.us-east-1.amazonaws.com" {
		t.Errorf("expected my-bucket.s3.us-east-1.amazonaws.com, got %s", host.Host)
	}
}

func TestS3Storage_HostURL_PathStyle(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:         "us-east-1",
		Bucket:         "my-bucket",
		Endpoint:       "http://localhost:9000",
		ForcePathStyle: true,
	})

	host := store.hostURL()
	if host.Host != "localhost:9000" {
		t.Errorf("expected localhost:9000, got %s", host.Host)
	}
}

func TestS3Storage_ObjectURL_VirtualHosted(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:   "us-east-1",
		Bucket:   "my-bucket",
		Endpoint: "s3.us-east-1.amazonaws.com",
	})

	objURL := store.objectURL("photos/img.jpg")
	expected := "https://my-bucket.s3.us-east-1.amazonaws.com/photos/img.jpg"
	if objURL != expected {
		t.Errorf("expected %s, got %s", expected, objURL)
	}
}

func TestS3Storage_ObjectURL_PathStyle(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:         "us-east-1",
		Bucket:         "my-bucket",
		Endpoint:       "http://localhost:9000",
		ForcePathStyle: true,
	})

	objURL := store.objectURL("photos/img.jpg")
	expected := "http://localhost:9000/my-bucket/photos/img.jpg"
	if objURL != expected {
		t.Errorf("expected %s, got %s", expected, objURL)
	}
}

func TestS3Storage_Put_NoCredentials(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:   "us-east-1",
		Bucket:   "test-bucket",
		Endpoint: "s3.us-east-1.amazonaws.com",
	})

	ctx := context.Background()
	_, err := store.Put(ctx, "test.txt", []byte("data"), StoragePutOptions{})
	if err == nil {
		t.Fatal("expected error with no credentials")
	}
}

func TestS3Storage_Get_NoCredentials(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:   "us-east-1",
		Bucket:   "test-bucket",
		Endpoint: "s3.us-east-1.amazonaws.com",
	})

	ctx := context.Background()
	_, err := store.Get(ctx, "test.txt")
	if err == nil {
		t.Fatal("expected error with no credentials")
	}
}

func TestS3Storage_Delete_NoCredentials(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:   "us-east-1",
		Bucket:   "test-bucket",
		Endpoint: "s3.us-east-1.amazonaws.com",
	})

	ctx := context.Background()
	err := store.Delete(ctx, "test.txt")
	if err == nil {
		t.Fatal("expected error with no credentials")
	}
}

func TestS3Storage_Exists_NoCredentials(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:   "us-east-1",
		Bucket:   "test-bucket",
		Endpoint: "s3.us-east-1.amazonaws.com",
	})

	ctx := context.Background()
	_, err := store.Exists(ctx, "test.txt")
	if err == nil {
		t.Fatal("expected error with no credentials")
	}
}

func TestS3Storage_List_NoCredentials(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:   "us-east-1",
		Bucket:   "test-bucket",
		Endpoint: "s3.us-east-1.amazonaws.com",
	})

	ctx := context.Background()
	_, err := store.List(ctx, "")
	if err == nil {
		t.Fatal("expected error with no credentials")
	}
}

func TestS3Storage_URL_Presigned(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:          "us-east-1",
		Bucket:          "my-bucket",
		Endpoint:        "s3.us-east-1.amazonaws.com",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	})

	ctx := context.Background()
	presignedURL, err := store.URL(ctx, "photos/img.jpg", time.Hour)
	if err != nil {
		t.Fatalf("presigned URL: %v", err)
	}

	if presignedURL == "" {
		t.Fatal("presigned URL is empty")
	}
}

func TestStorageManager_RegisterAndGet(t *testing.T) {
	dir := t.TempDir()
	sm := NewStorageManager()

	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})
	sm.Register(local)

	got := sm.Backend(StorageLocal)
	if got != local {
		t.Error("expected local backend")
	}
}

func TestStorageManager_Default(t *testing.T) {
	dir := t.TempDir()
	sm := NewStorageManager()

	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})
	sm.Register(local)

	if sm.Default() != local {
		t.Error("expected local as default")
	}
}

func TestStorageManager_SetDefault(t *testing.T) {
	dir := t.TempDir()
	sm := NewStorageManager()

	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})
	sm.Register(local)

	s3 := NewS3Storage(S3Config{
		Region:   "us-east-1",
		Bucket:   "test",
		Endpoint: "s3.us-east-1.amazonaws.com",
	})
	sm.Register(s3)
	sm.SetDefault(StorageS3)

	if sm.Default() != s3 {
		t.Error("expected S3 as default")
	}
}

func TestStorageManager_NoBackend(t *testing.T) {
	sm := NewStorageManager()

	ctx := context.Background()
	_, err := sm.Put(ctx, "key", []byte("data"), StoragePutOptions{})
	if err == nil {
		t.Fatal("expected error with no backend")
	}

	_, err = sm.Get(ctx, "key")
	if err == nil {
		t.Fatal("expected error with no backend")
	}

	err = sm.Delete(ctx, "key")
	if err == nil {
		t.Fatal("expected error with no backend")
	}

	_, err = sm.Exists(ctx, "key")
	if err == nil {
		t.Fatal("expected error with no backend")
	}

	_, err = sm.List(ctx, "")
	if err == nil {
		t.Fatal("expected error with no backend")
	}

	_, err = sm.URL(ctx, "key", 0)
	if err == nil {
		t.Fatal("expected error with no backend")
	}
}

func TestProcessor_SetStorage(t *testing.T) {
	dir := t.TempDir()
	proc := NewProcessor("")

	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})
	proc.SetStorage(local)

	if proc.Storage() != local {
		t.Error("storage not set")
	}
}

func TestProcessor_Upload_LocalStorage(t *testing.T) {
	dir := t.TempDir()
	proc := NewProcessor("")

	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})
	proc.SetStorage(local)

	ctx := context.Background()
	media := &Media{
		ID:       "upload-test",
		Type:     TypeImage,
		Data:     []byte("image data"),
		MimeType: "image/jpeg",
		Name:     "test.jpg",
	}

	url, err := proc.Upload(ctx, media)
	if err != nil {
		t.Fatalf("upload: %v", err)
	}

	if url == "" {
		t.Error("upload returned empty URL")
	}

	if media.URL != url {
		t.Errorf("media URL not updated")
	}
}

func TestProcessor_Save_LocalStorage(t *testing.T) {
	dir := t.TempDir()
	proc := NewProcessor("")

	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})
	proc.SetStorage(local)

	ctx := context.Background()
	media := &Media{
		ID:       "save-test",
		Type:     TypeDoc,
		Data:     []byte("document content"),
		MimeType: "application/pdf",
		Name:     "report.pdf",
	}

	url, err := proc.Save(ctx, media)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	if url == "" {
		t.Error("save returned empty URL")
	}
}

func TestProcessor_Load_LocalStorage(t *testing.T) {
	dir := t.TempDir()
	proc := NewProcessor("")

	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})
	proc.SetStorage(local)

	ctx := context.Background()
	original := &Media{
		ID:       "load-test",
		Type:     TypeDoc,
		Data:     []byte("loadable content"),
		MimeType: "text/plain",
		Name:     "load.txt",
	}

	_, err := proc.Save(ctx, original)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := proc.Load(ctx, "load.txt")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if string(loaded.Data) != "loadable content" {
		t.Errorf("expected loadable content, got %s", string(loaded.Data))
	}
}

func TestProcessor_Delete_LocalStorage(t *testing.T) {
	dir := t.TempDir()
	proc := NewProcessor("")

	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})
	proc.SetStorage(local)

	ctx := context.Background()
	media := &Media{
		ID:   "delete-test",
		Data: []byte("to delete"),
		Name: "delete.txt",
	}

	_, err := proc.Save(ctx, media)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	err = proc.Delete(ctx, "delete.txt")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	exists, err := proc.Exists(ctx, "delete.txt")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists {
		t.Error("expected file to be deleted")
	}
}

func TestProcessor_List_LocalStorage(t *testing.T) {
	dir := t.TempDir()
	proc := NewProcessor("")

	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})
	proc.SetStorage(local)

	ctx := context.Background()
	_, _ = proc.Save(ctx, &Media{ID: "1", Data: []byte("a"), Name: "a.txt"})
	_, _ = proc.Save(ctx, &Media{ID: "2", Data: []byte("b"), Name: "b.txt"})

	objects, err := proc.List(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(objects) < 2 {
		t.Errorf("expected at least 2 objects, got %d", len(objects))
	}
}

func TestProcessor_PresignURL_LocalStorage(t *testing.T) {
	dir := t.TempDir()
	proc := NewProcessor("")

	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://example.com",
	})
	proc.SetStorage(local)

	ctx := context.Background()
	media := &Media{
		ID:   "presign-test",
		Data: []byte("presignable"),
		Name: "presign.txt",
	}

	_, err := proc.Save(ctx, media)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	presignedURL, err := proc.PresignURL(ctx, "presign.txt", time.Hour)
	if err != nil {
		t.Fatalf("presign: %v", err)
	}

	if presignedURL != "https://example.com/presign.txt" {
		t.Errorf("expected https://example.com/presign.txt, got %s", presignedURL)
	}
}

func TestProcessor_NoStorageBackend(t *testing.T) {
	proc := NewProcessor("")

	ctx := context.Background()
	media := &Media{
		ID:   "no-storage",
		Data: []byte("data"),
	}

	_, err := proc.Upload(ctx, media)
	if err == nil {
		t.Fatal("expected error with no storage")
	}

	_, err = proc.Save(ctx, media)
	if err == nil {
		t.Fatal("expected error with no storage")
	}

	_, err = proc.Load(ctx, "key")
	if err == nil {
		t.Fatal("expected error with no storage")
	}

	err = proc.Delete(ctx, "key")
	if err == nil {
		t.Fatal("expected error with no storage")
	}

	_, err = proc.Exists(ctx, "key")
	if err == nil {
		t.Fatal("expected error with no storage")
	}

	_, err = proc.List(ctx, "")
	if err == nil {
		t.Fatal("expected error with no storage")
	}

	_, err = proc.PresignURL(ctx, "key", 0)
	if err == nil {
		t.Fatal("expected error with no storage")
	}
}

func TestMediaPipeline_SaveToStorage(t *testing.T) {
	dir := t.TempDir()
	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})

	cfg := DefaultMediaPipelineConfig()
	cfg.Storage = local

	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	media := &Media{
		ID:       "pipeline-save",
		Type:     TypeImage,
		Data:     []byte("image data"),
		MimeType: "image/jpeg",
	}

	url, err := p.SaveToStorage(ctx, media)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	if url == "" {
		t.Error("save returned empty URL")
	}
}

func TestMediaPipeline_LoadFromStorage(t *testing.T) {
	dir := t.TempDir()
	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})

	cfg := DefaultMediaPipelineConfig()
	cfg.Storage = local

	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	media := &Media{
		ID:       "pipeline-load",
		Type:     TypeDoc,
		Data:     []byte("loadable data"),
		MimeType: "text/plain",
	}

	_, err := p.SaveToStorage(ctx, media)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := p.LoadFromStorage(ctx, media.ID+".bin")
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if string(loaded.Data) != "loadable data" {
		t.Errorf("expected loadable data, got %s", string(loaded.Data))
	}
}

func TestMediaPipeline_DeleteFromStorage(t *testing.T) {
	dir := t.TempDir()
	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})

	cfg := DefaultMediaPipelineConfig()
	cfg.Storage = local

	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	media := &Media{
		ID:   "pipeline-delete",
		Data: []byte("delete me"),
	}

	_, err := p.SaveToStorage(ctx, media)
	if err != nil {
		t.Fatalf("save: %v", err)
	}

	err = p.DeleteFromStorage(ctx, media.ID+".bin")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestMediaPipeline_AutoSave(t *testing.T) {
	dir := t.TempDir()
	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})

	cfg := DefaultMediaPipelineConfig()
	cfg.Storage = local
	cfg.AutoSave = true

	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	_, err := p.Download(ctx, "http://localhost:1/media.txt")
	if err == nil {
		t.Log("auto-save attempted (expected failure due to no server)")
	}
}

func TestMediaPipeline_StorageURL(t *testing.T) {
	dir := t.TempDir()
	local := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://cdn.example.com",
	})

	cfg := DefaultMediaPipelineConfig()
	cfg.Storage = local

	p := NewMediaPipeline(cfg)

	ctx := context.Background()
	objURL, err := p.StorageURL(ctx, "photo.jpg", 0)
	if err != nil {
		t.Fatalf("storage URL: %v", err)
	}

	expected := "https://cdn.example.com/photo.jpg"
	if objURL != expected {
		t.Errorf("expected %s, got %s", expected, objURL)
	}
}

func TestMediaPipeline_NoStorageConfigured(t *testing.T) {
	cfg := DefaultMediaPipelineConfig()
	p := NewMediaPipeline(cfg)

	ctx := context.Background()

	_, err := p.SaveToStorage(ctx, &Media{})
	if err == nil {
		t.Fatal("expected error with no storage")
	}

	_, err = p.LoadFromStorage(ctx, "key")
	if err == nil {
		t.Fatal("expected error with no storage")
	}

	err = p.DeleteFromStorage(ctx, "key")
	if err == nil {
		t.Fatal("expected error with no storage")
	}

	_, err = p.StorageURL(ctx, "key", 0)
	if err == nil {
		t.Fatal("expected error with no storage")
	}
}

func TestLocalStorage_PutCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()
	_, err := store.Put(ctx, "deep/nested/path/file.txt", []byte("data"), StoragePutOptions{})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	exists, err := store.Exists(ctx, "deep/nested/path/file.txt")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if !exists {
		t.Error("expected file to exist in nested directory")
	}
}

func TestLocalStorage_GetNotFound(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()
	_, err := store.Get(ctx, "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLocalStorage_ListEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()
	objects, err := store.List(ctx, "")
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(objects) != 0 {
		t.Errorf("expected 0 objects, got %d", len(objects))
	}
}

func TestLocalStorage_ListWithPrefix(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()
	_, _ = store.Put(ctx, "img_photo1.jpg", []byte("1"), StoragePutOptions{})
	_, _ = store.Put(ctx, "img_photo2.jpg", []byte("2"), StoragePutOptions{})
	_, _ = store.Put(ctx, "doc_report.pdf", []byte("3"), StoragePutOptions{})

	objects, err := store.List(ctx, "img")
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	if len(objects) != 2 {
		t.Errorf("expected 2 objects with img prefix, got %d", len(objects))
	}
}

func TestStorageManager_PutGetDelete(t *testing.T) {
	dir := t.TempDir()
	sm := NewStorageManager()

	local := NewLocalStorage(LocalStorageConfig{BasePath: dir})
	sm.Register(local)

	ctx := context.Background()

	obj, err := sm.Put(ctx, "manager-test.txt", []byte("manager data"), StoragePutOptions{
		MimeType: "text/plain",
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	if obj.Key != "manager-test.txt" {
		t.Errorf("expected key manager-test.txt, got %s", obj.Key)
	}

	got, err := sm.Get(ctx, "manager-test.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	if got.Key != "manager-test.txt" {
		t.Errorf("expected key manager-test.txt, got %s", got.Key)
	}

	err = sm.Delete(ctx, "manager-test.txt")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	exists, err := sm.Exists(ctx, "manager-test.txt")
	if err != nil {
		t.Fatalf("exists: %v", err)
	}
	if exists {
		t.Error("expected file to be deleted")
	}
}

func TestS3Storage_PutWithSessionToken(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:          "us-east-1",
		Bucket:          "test-bucket",
		Endpoint:        "s3.us-east-1.amazonaws.com",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBY...",
	})

	ctx := context.Background()
	_, err := store.Put(ctx, "test.txt", []byte("data"), StoragePutOptions{})
	if err == nil {
		t.Fatal("expected error (no real S3)")
	}
}

func TestS3Storage_PutWithACL(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:          "us-east-1",
		Bucket:          "test-bucket",
		Endpoint:        "s3.us-east-1.amazonaws.com",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	})

	ctx := context.Background()
	_, err := store.Put(ctx, "acl-test.txt", []byte("data"), StoragePutOptions{
		ACL: "private",
	})
	if err == nil {
		t.Fatal("expected error (no real S3)")
	}
}

func TestS3Storage_PutWithCacheControl(t *testing.T) {
	store := NewS3Storage(S3Config{
		Region:          "us-east-1",
		Bucket:          "test-bucket",
		Endpoint:        "s3.us-east-1.amazonaws.com",
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
	})

	ctx := context.Background()
	_, err := store.Put(ctx, "cache-test.txt", []byte("data"), StoragePutOptions{
		CacheControl: "max-age=3600",
	})
	if err == nil {
		t.Fatal("expected error (no real S3)")
	}
}

func TestLocalStorage_BaseURL_TrailingSlash(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
		BaseURL:  "https://example.com/media/",
	})

	ctx := context.Background()
	objURL, err := store.URL(ctx, "photo.jpg", 0)
	if err != nil {
		t.Fatalf("url: %v", err)
	}

	expected := "https://example.com/media/photo.jpg"
	if objURL != expected {
		t.Errorf("expected %s, got %s", expected, objURL)
	}
}

func TestLocalStorage_GetDataIntegrity(t *testing.T) {
	dir := t.TempDir()
	store := NewLocalStorage(LocalStorageConfig{
		BasePath: dir,
	})

	ctx := context.Background()
	original := []byte("integrity test content with special chars: !@#$%^&*()")

	_, err := store.Put(ctx, "integrity.txt", original, StoragePutOptions{})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	obj, err := store.Get(ctx, "integrity.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	retrieved, err := os.ReadFile(filepath.Join(dir, "integrity.txt"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if string(retrieved) != string(original) {
		t.Errorf("data integrity check failed")
	}

	_ = obj
}
