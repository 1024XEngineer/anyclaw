package media

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type StorageBackendType string

const (
	StorageLocal StorageBackendType = "local"
	StorageS3    StorageBackendType = "s3"
)

type StorageObject struct {
	Key          string
	Size         int64
	MimeType     string
	LastModified time.Time
	URL          string
	Metadata     map[string]string
}

type StorageBackend interface {
	Type() StorageBackendType
	Put(ctx context.Context, key string, data []byte, opts StoragePutOptions) (*StorageObject, error)
	Get(ctx context.Context, key string) (*StorageObject, error)
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	List(ctx context.Context, prefix string) ([]*StorageObject, error)
	URL(ctx context.Context, key string, expires time.Duration) (string, error)
}

type StoragePutOptions struct {
	MimeType     string
	Metadata     map[string]string
	ACL          string
	CacheControl string
}

type LocalStorageConfig struct {
	BasePath string
	BaseURL  string
}

type LocalStorage struct {
	mu       sync.RWMutex
	basePath string
	baseURL  string
}

func NewLocalStorage(cfg LocalStorageConfig) *LocalStorage {
	return &LocalStorage{
		basePath: cfg.BasePath,
		baseURL:  cfg.BaseURL,
	}
}

func (s *LocalStorage) Type() StorageBackendType {
	return StorageLocal
}

func (s *LocalStorage) Put(ctx context.Context, key string, data []byte, opts StoragePutOptions) (*StorageObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.basePath, key)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file: %w", err)
	}

	obj := &StorageObject{
		Key:          key,
		Size:         info.Size(),
		MimeType:     opts.MimeType,
		LastModified: info.ModTime(),
		Metadata:     opts.Metadata,
	}

	if s.baseURL != "" {
		obj.URL = strings.TrimRight(s.baseURL, "/") + "/" + key
	} else {
		obj.URL = "file://" + path
	}

	return obj, nil
}

func (s *LocalStorage) Get(ctx context.Context, key string) (*StorageObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.basePath, key)

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("object not found: %s", key)
		}
		return nil, fmt.Errorf("stat file: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	mimeType := mime.TypeByExtension(filepath.Ext(key))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	obj := &StorageObject{
		Key:          key,
		Size:         info.Size(),
		MimeType:     mimeType,
		LastModified: info.ModTime(),
		Metadata:     map[string]string{"data": base64.StdEncoding.EncodeToString(data)},
	}

	if s.baseURL != "" {
		obj.URL = strings.TrimRight(s.baseURL, "/") + "/" + key
	} else {
		obj.URL = "file://" + path
	}

	return obj, nil
}

func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := filepath.Join(s.basePath, key)

	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("object not found: %s", key)
		}
		return fmt.Errorf("delete file: %w", err)
	}

	return nil
}

func (s *LocalStorage) Exists(ctx context.Context, key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.basePath, key)

	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat file: %w", err)
	}

	return true, nil
}

func (s *LocalStorage) List(ctx context.Context, prefix string) ([]*StorageObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	searchDir := s.basePath
	if prefix != "" {
		searchDir = filepath.Join(s.basePath, filepath.Dir(prefix))
	}

	entries, err := os.ReadDir(searchDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var objects []*StorageObject
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if prefix != "" {
			basePrefix := filepath.Base(prefix)
			if basePrefix != "." && !strings.HasPrefix(name, basePrefix) {
				continue
			}
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		key := name
		if prefix != "" {
			key = filepath.Join(filepath.Dir(prefix), name)
		}
		objURL := ""
		if s.baseURL != "" {
			objURL = strings.TrimRight(s.baseURL, "/") + "/" + key
		}

		objects = append(objects, &StorageObject{
			Key:          key,
			Size:         info.Size(),
			LastModified: info.ModTime(),
			URL:          objURL,
		})
	}

	return objects, nil
}

func (s *LocalStorage) URL(ctx context.Context, key string, expires time.Duration) (string, error) {
	if s.baseURL != "" {
		return strings.TrimRight(s.baseURL, "/") + "/" + key, nil
	}
	return "file://" + filepath.Join(s.basePath, key), nil
}

type S3Config struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	UsePathStyle    bool
	ForcePathStyle  bool
	PublicRead      bool
}

type S3Storage struct {
	mu     sync.RWMutex
	cfg    S3Config
	client *http.Client
}

func NewS3Storage(cfg S3Config) *S3Storage {
	endpoint := cfg.Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("s3.%s.amazonaws.com", cfg.Region)
	}

	return &S3Storage{
		cfg: cfg,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (s *S3Storage) Type() StorageBackendType {
	return StorageS3
}

func (s *S3Storage) host() string {
	if s.cfg.ForcePathStyle || s.cfg.UsePathStyle {
		endpoint := s.cfg.Endpoint
		if !strings.HasPrefix(endpoint, "http") {
			endpoint = "https://" + endpoint
		}
		return endpoint
	}
	return fmt.Sprintf("https://%s.%s", s.cfg.Bucket, s.cfg.Endpoint)
}

func (s *S3Storage) objectURL(key string) string {
	if s.cfg.ForcePathStyle || s.cfg.UsePathStyle {
		endpoint := strings.TrimRight(s.cfg.Endpoint, "/")
		if !strings.HasPrefix(endpoint, "http") {
			endpoint = "https://" + endpoint
		}
		return fmt.Sprintf("%s/%s/%s", endpoint, s.cfg.Bucket, key)
	}
	return fmt.Sprintf("https://%s.%s/%s", s.cfg.Bucket, s.cfg.Endpoint, key)
}

func (s *S3Storage) Put(ctx context.Context, key string, data []byte, opts StoragePutOptions) (*StorageObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	method := "PUT"
	contentType := opts.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	bodyHash := sha256Hex(data)

	headers := map[string]string{
		"Host":                 s.hostURL().Host,
		"Content-Type":         contentType,
		"Content-Length":       fmt.Sprintf("%d", len(data)),
		"X-Amz-Content-Sha256": bodyHash,
		"X-Amz-Date":           time.Now().UTC().Format("20060102T150405Z"),
	}

	if opts.CacheControl != "" {
		headers["Cache-Control"] = opts.CacheControl
	}

	if s.cfg.PublicRead {
		headers["X-Amz-Acl"] = "public-read"
	} else if opts.ACL != "" {
		headers["X-Amz-Acl"] = opts.ACL
	}

	for k, v := range opts.Metadata {
		headers["X-Amz-Meta-"+k] = v
	}

	if s.cfg.SessionToken != "" {
		headers["X-Amz-Security-Token"] = s.cfg.SessionToken
	}

	path := "/" + s.cfg.Bucket + "/" + key
	authHeader := s.signV4(method, path, "", headers, bodyHash)

	reqURL := s.objectURL(key)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("put request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("s3 put failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	obj := &StorageObject{
		Key:          key,
		Size:         int64(len(data)),
		MimeType:     contentType,
		LastModified: time.Now(),
		URL:          s.objectURL(key),
		Metadata:     opts.Metadata,
	}

	return obj, nil
}

func (s *S3Storage) Get(ctx context.Context, key string) (*StorageObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	method := "GET"
	bodyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	headers := map[string]string{
		"Host":                 s.hostURL().Host,
		"X-Amz-Content-Sha256": bodyHash,
		"X-Amz-Date":           time.Now().UTC().Format("20060102T150405Z"),
	}

	if s.cfg.SessionToken != "" {
		headers["X-Amz-Security-Token"] = s.cfg.SessionToken
	}

	path := "/" + s.cfg.Bucket + "/" + key
	authHeader := s.signV4(method, path, "", headers, bodyHash)

	reqURL := s.objectURL(key)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("s3 get failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	metadata := make(map[string]string)
	for k, v := range resp.Header {
		if strings.HasPrefix(k, "X-Amz-Meta-") {
			metadata[strings.TrimPrefix(k, "X-Amz-Meta-")] = v[0]
		}
	}

	obj := &StorageObject{
		Key:      key,
		Size:     resp.ContentLength,
		MimeType: resp.Header.Get("Content-Type"),
		URL:      s.objectURL(key),
		Metadata: metadata,
	}

	if resp.Header.Get("Last-Modified") != "" {
		if t, err := time.Parse(time.RFC1123, resp.Header.Get("Last-Modified")); err == nil {
			obj.LastModified = t
		}
	}

	obj.Metadata["data"] = base64.StdEncoding.EncodeToString(data)

	return obj, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	method := "DELETE"
	bodyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	headers := map[string]string{
		"Host":                 s.hostURL().Host,
		"X-Amz-Content-Sha256": bodyHash,
		"X-Amz-Date":           time.Now().UTC().Format("20060102T150405Z"),
	}

	if s.cfg.SessionToken != "" {
		headers["X-Amz-Security-Token"] = s.cfg.SessionToken
	}

	path := "/" + s.cfg.Bucket + "/" + key
	authHeader := s.signV4(method, path, "", headers, bodyHash)

	reqURL := s.objectURL(key)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("delete request: %w", err)
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 300 && resp.StatusCode != 404 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("s3 delete failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (s *S3Storage) Exists(ctx context.Context, key string) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	method := "HEAD"
	bodyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	headers := map[string]string{
		"Host":                 s.hostURL().Host,
		"X-Amz-Content-Sha256": bodyHash,
		"X-Amz-Date":           time.Now().UTC().Format("20060102T150405Z"),
	}

	if s.cfg.SessionToken != "" {
		headers["X-Amz-Security-Token"] = s.cfg.SessionToken
	}

	path := "/" + s.cfg.Bucket + "/" + key
	authHeader := s.signV4(method, path, "", headers, bodyHash)

	reqURL := s.objectURL(key)

	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return false, fmt.Errorf("create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := s.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("head request: %w", err)
	}
	defer resp.Body.Close()

	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == 404 {
		return false, nil
	}
	if resp.StatusCode >= 300 {
		return false, fmt.Errorf("s3 head failed: HTTP %d", resp.StatusCode)
	}

	return true, nil
}

func (s *S3Storage) List(ctx context.Context, prefix string) ([]*StorageObject, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	method := "GET"
	bodyHash := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	query := url.Values{}
	if prefix != "" {
		query.Set("prefix", prefix)
	}
	query.Set("max-keys", "1000")
	queryStr := query.Encode()

	headers := map[string]string{
		"Host":                 s.hostURL().Host,
		"X-Amz-Content-Sha256": bodyHash,
		"X-Amz-Date":           time.Now().UTC().Format("20060102T150405Z"),
	}

	if s.cfg.SessionToken != "" {
		headers["X-Amz-Security-Token"] = s.cfg.SessionToken
	}

	path := "/" + s.cfg.Bucket + "/"
	authHeader := s.signV4(method, path, queryStr, headers, bodyHash)

	var reqURL string
	if s.cfg.ForcePathStyle || s.cfg.UsePathStyle {
		endpoint := strings.TrimRight(s.cfg.Endpoint, "/")
		if !strings.HasPrefix(endpoint, "http") {
			endpoint = "https://" + endpoint
		}
		reqURL = fmt.Sprintf("%s/%s?%s", endpoint, s.cfg.Bucket, queryStr)
	} else {
		reqURL = fmt.Sprintf("https://%s.%s/?%s", s.cfg.Bucket, s.cfg.Endpoint, queryStr)
	}

	req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("s3 list failed: HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return parseS3ListResponse(body, s.objectURL(""))
}

func (s *S3Storage) URL(ctx context.Context, key string, expires time.Duration) (string, error) {
	if expires <= 0 {
		return s.objectURL(key), nil
	}

	now := time.Now().UTC()

	query := url.Values{}
	query.Set("X-Amz-Algorithm", "AWS4-HMAC-SHA256")
	query.Set("X-Amz-Credential", fmt.Sprintf("%s/%s/%s/s3/aws4_request", s.cfg.AccessKeyID, now.Format("20060102"), s.cfg.Region))
	query.Set("X-Amz-Date", now.Format("20060102T150405Z"))
	query.Set("X-Amz-Expires", fmt.Sprintf("%d", int(expires.Seconds())))
	query.Set("X-Amz-SignedHeaders", "host")

	if s.cfg.SessionToken != "" {
		query.Set("X-Amz-Security-Token", s.cfg.SessionToken)
	}

	stringToSign := s.presignStringToSign(methodGET, "/"+s.cfg.Bucket+"/"+key, query.Encode(), now.Format("20060102"))
	signingKey := s.deriveSigningKey(now.Format("20060102"))
	signature := hmacSHA256(signingKey, stringToSign)

	query.Set("X-Amz-Signature", hex.EncodeToString(signature))

	var reqURL string
	if s.cfg.ForcePathStyle || s.cfg.UsePathStyle {
		endpoint := strings.TrimRight(s.cfg.Endpoint, "/")
		if !strings.HasPrefix(endpoint, "http") {
			endpoint = "https://" + endpoint
		}
		reqURL = fmt.Sprintf("%s/%s/%s?%s", endpoint, s.cfg.Bucket, url.PathEscape(key), query.Encode())
	} else {
		reqURL = fmt.Sprintf("https://%s.%s/%s?%s", s.cfg.Bucket, s.cfg.Endpoint, url.PathEscape(key), query.Encode())
	}

	return reqURL, nil
}

const methodGET = "GET"

func (s *S3Storage) hostURL() *url.URL {
	h := s.host()
	if !strings.HasPrefix(h, "http") {
		h = "https://" + h
	}
	u, _ := url.Parse(h)
	return u
}

func (s *S3Storage) signV4(method, path, query string, headers map[string]string, bodyHash string) string {
	now := time.Now().UTC()
	dateStamp := now.Format("20060102")

	signedHeaders := make([]string, 0, len(headers))
	for k := range headers {
		lower := strings.ToLower(k)
		if lower == "authorization" {
			continue
		}
		signedHeaders = append(signedHeaders, lower)
	}
	sort.Strings(signedHeaders)

	canonicalHeaders := ""
	for _, h := range signedHeaders {
		canonicalHeaders += h + ":" + strings.TrimSpace(headers[headerKey(h)]) + "\n"
	}

	canonicalRequest := method + "\n" + path + "\n" + query + "\n" + canonicalHeaders + "\n" + strings.Join(signedHeaders, ";") + "\n" + bodyHash

	stringToSign := "AWS4-HMAC-SHA256\n" + now.Format("20060102T150405Z") + "\n" + dateStamp + "/" + s.cfg.Region + "/s3/aws4_request\n" + sha256HexString(canonicalRequest)

	signingKey := s.deriveSigningKey(dateStamp)
	signature := hmacSHA256(signingKey, stringToSign)

	return "AWS4-HMAC-SHA256 Credential=" + s.cfg.AccessKeyID + "/" + dateStamp + "/" + s.cfg.Region + "/s3/aws4_request, SignedHeaders=" + strings.Join(signedHeaders, ";") + ", Signature=" + hex.EncodeToString(signature)
}

func (s *S3Storage) presignStringToSign(method, path, query, dateStamp string) string {
	return "AWS4-HMAC-SHA256\n" + dateStamp + "T000000Z\n" + dateStamp + "/" + s.cfg.Region + "/s3/aws4_request\n" + sha256HexString(method+"\n"+path+"\n"+query+"\nhost:"+s.hostURL().Host+"\n\nhost\nUNSIGNED-PAYLOAD")
}

func (s *S3Storage) deriveSigningKey(dateStamp string) []byte {
	kSecret := []byte("AWS4" + s.cfg.SecretAccessKey)
	kDate := hmacSHA256(kSecret, dateStamp)
	kRegion := hmacSHA256(kDate, s.cfg.Region)
	kService := hmacSHA256(kRegion, "s3")
	return hmacSHA256(kService, "aws4_request")
}

func headerKey(lower string) string {
	switch lower {
	case "host":
		return "Host"
	case "content-type":
		return "Content-Type"
	case "content-length":
		return "Content-Length"
	case "x-amz-content-sha256":
		return "X-Amz-Content-Sha256"
	case "x-amz-date":
		return "X-Amz-Date"
	case "cache-control":
		return "Cache-Control"
	case "x-amz-acl":
		return "X-Amz-Acl"
	case "x-amz-security-token":
		return "X-Amz-Security-Token"
	default:
		if strings.HasPrefix(lower, "x-amz-meta-") {
			return "X-Amz-Meta-" + strings.TrimPrefix(lower, "x-amz-meta-")
		}
		return lower
	}
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func sha256HexString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

type s3ListResponse struct {
	Contents []s3ListContents `json:"Contents"`
}

type s3ListContents struct {
	Key          string `json:"Key"`
	Size         int64  `json:"Size"`
	LastModified string `json:"LastModified"`
}

func parseS3ListResponse(body []byte, baseURL string) ([]*StorageObject, error) {
	var result s3ListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return parseS3ListXML(body, baseURL)
	}

	objects := make([]*StorageObject, 0, len(result.Contents))
	for _, c := range result.Contents {
		objURL := baseURL + c.Key
		objects = append(objects, &StorageObject{
			Key:  c.Key,
			Size: c.Size,
			URL:  objURL,
		})
	}

	return objects, nil
}

func parseS3ListXML(body []byte, baseURL string) ([]*StorageObject, error) {
	var objects []*StorageObject

	contentTag := "<Contents>"
	endTag := "</Contents>"
	keyTag := "<Key>"
	keyEndTag := "</Key>"
	sizeTag := "<Size>"
	sizeEndTag := "</Size>"
	lmTag := "<LastModified>"
	lmEndTag := "</LastModified>"

	pos := 0
	for {
		idx := bytes.Index(body[pos:], []byte(contentTag))
		if idx == -1 {
			break
		}
		start := pos + idx + len(contentTag)
		endIdx := bytes.Index(body[start:], []byte(endTag))
		if endIdx == -1 {
			break
		}
		content := body[start : start+endIdx]

		obj := &StorageObject{URL: baseURL}

		keyIdx := bytes.Index(content, []byte(keyTag))
		if keyIdx >= 0 {
			keyStart := keyIdx + len(keyTag)
			keyEnd := bytes.Index(content[keyStart:], []byte(keyEndTag))
			if keyEnd >= 0 {
				obj.Key = string(content[keyStart : keyStart+keyEnd])
			}
		}

		sizeIdx := bytes.Index(content, []byte(sizeTag))
		if sizeIdx >= 0 {
			sizeStart := sizeIdx + len(sizeTag)
			sizeEnd := bytes.Index(content[sizeStart:], []byte(sizeEndTag))
			if sizeEnd >= 0 {
				fmt.Sscanf(string(content[sizeStart:sizeStart+sizeEnd]), "%d", &obj.Size)
			}
		}

		lmIdx := bytes.Index(content, []byte(lmTag))
		if lmIdx >= 0 {
			lmStart := lmIdx + len(lmTag)
			lmEnd := bytes.Index(content[lmStart:], []byte(lmEndTag))
			if lmEnd >= 0 {
				if t, err := time.Parse(time.RFC3339, string(content[lmStart:lmStart+lmEnd])); err == nil {
					obj.LastModified = t
				}
			}
		}

		if obj.Key != "" {
			objects = append(objects, obj)
		}

		pos = start + endIdx + len(endTag)
	}

	return objects, nil
}

type StorageManager struct {
	mu        sync.RWMutex
	backends  map[StorageBackendType]StorageBackend
	defaultBE StorageBackendType
}

func NewStorageManager() *StorageManager {
	return &StorageManager{
		backends: make(map[StorageBackendType]StorageBackend),
	}
}

func (sm *StorageManager) Register(backend StorageBackend) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.backends[backend.Type()] = backend
	if sm.defaultBE == "" {
		sm.defaultBE = backend.Type()
	}
}

func (sm *StorageManager) SetDefault(t StorageBackendType) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.defaultBE = t
}

func (sm *StorageManager) Backend(t StorageBackendType) StorageBackend {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.backends[t]
}

func (sm *StorageManager) Default() StorageBackend {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.backends[sm.defaultBE]
}

func (sm *StorageManager) Put(ctx context.Context, key string, data []byte, opts StoragePutOptions) (*StorageObject, error) {
	be := sm.Default()
	if be == nil {
		return nil, fmt.Errorf("no storage backend configured")
	}
	return be.Put(ctx, key, data, opts)
}

func (sm *StorageManager) Get(ctx context.Context, key string) (*StorageObject, error) {
	be := sm.Default()
	if be == nil {
		return nil, fmt.Errorf("no storage backend configured")
	}
	return be.Get(ctx, key)
}

func (sm *StorageManager) Delete(ctx context.Context, key string) error {
	be := sm.Default()
	if be == nil {
		return fmt.Errorf("no storage backend configured")
	}
	return be.Delete(ctx, key)
}

func (sm *StorageManager) Exists(ctx context.Context, key string) (bool, error) {
	be := sm.Default()
	if be == nil {
		return false, fmt.Errorf("no storage backend configured")
	}
	return be.Exists(ctx, key)
}

func (sm *StorageManager) List(ctx context.Context, prefix string) ([]*StorageObject, error) {
	be := sm.Default()
	if be == nil {
		return nil, fmt.Errorf("no storage backend configured")
	}
	return be.List(ctx, prefix)
}

func (sm *StorageManager) URL(ctx context.Context, key string, expires time.Duration) (string, error) {
	be := sm.Default()
	if be == nil {
		return "", fmt.Errorf("no storage backend configured")
	}
	return be.URL(ctx, key, expires)
}
