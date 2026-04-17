package secrets

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type AuditQuery struct {
	Operation  Operation
	Actor      string
	SecretKey  string
	SnapshotID string
	LockID     string
	StartTime  time.Time
	EndTime    time.Time
	Success    *bool
	Limit      int
}

type AuditSummary struct {
	TotalOperations    int            `json:"total_operations"`
	OperationsByType   map[string]int `json:"operations_by_type"`
	TopActors          map[string]int `json:"top_actors"`
	SuccessRate        float64        `json:"success_rate"`
	RecentFailures     int            `json:"recent_failures"`
	SecretsAccessCount map[string]int `json:"secrets_access_count"`
}

type AuditReporter struct {
	store *Store
}

func NewAuditReporter(store *Store) *AuditReporter {
	return &AuditReporter{store: store}
}

func (ar *AuditReporter) Query(query *AuditQuery) []*AuditEntry {
	entries := ar.store.ListAuditEntries(0)
	if query == nil {
		return entries
	}

	filtered := make([]*AuditEntry, 0, len(entries))
	for _, entry := range entries {
		if !ar.matchEntry(entry, query) {
			continue
		}
		filtered = append(filtered, entry)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp.After(filtered[j].Timestamp)
	})

	if query.Limit > 0 && len(filtered) > query.Limit {
		filtered = filtered[:query.Limit]
	}

	return filtered
}

func (ar *AuditReporter) Summary(since time.Time) *AuditSummary {
	entries := ar.store.ListAuditEntries(0)

	summary := &AuditSummary{
		OperationsByType:   make(map[string]int),
		TopActors:          make(map[string]int),
		SecretsAccessCount: make(map[string]int),
	}

	var successCount int
	var recentFailures int

	for _, entry := range entries {
		if !since.IsZero() && entry.Timestamp.Before(since) {
			continue
		}

		summary.TotalOperations++
		summary.OperationsByType[string(entry.Operation)]++
		summary.TopActors[entry.Actor]++

		if entry.Success {
			successCount++
		} else {
			recentFailures++
		}

		if entry.Operation == OpAccess && entry.SecretKey != "" {
			summary.SecretsAccessCount[entry.SecretKey]++
		}
	}

	if summary.TotalOperations > 0 {
		summary.SuccessRate = float64(successCount) / float64(summary.TotalOperations) * 100
	}
	summary.RecentFailures = recentFailures

	return summary
}

func (ar *AuditReporter) SecretAccessHistory(secretKey string, limit int) []*AuditEntry {
	query := &AuditQuery{
		Operation: OpAccess,
		SecretKey: secretKey,
		Limit:     limit,
	}
	return ar.Query(query)
}

func (ar *AuditReporter) ActorActivity(actor string, since time.Time) []*AuditEntry {
	query := &AuditQuery{
		Actor:     actor,
		StartTime: since,
		Limit:     100,
	}
	return ar.Query(query)
}

func (ar *AuditReporter) FailedOperations(since time.Time, limit int) []*AuditEntry {
	success := false
	query := &AuditQuery{
		Success:   &success,
		StartTime: since,
		Limit:     limit,
	}
	return ar.Query(query)
}

func (ar *AuditReporter) ExportCSV(entries []*AuditEntry) string {
	var sb strings.Builder
	sb.WriteString("timestamp,operation,secret_key,snapshot_id,lock_id,actor,ip,success,error\n")

	for _, entry := range entries {
		sb.WriteString(fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%t,%s\n",
			entry.Timestamp.Format(time.RFC3339),
			entry.Operation,
			escapeCSV(entry.SecretKey),
			escapeCSV(entry.SnapshotID),
			escapeCSV(entry.LockID),
			escapeCSV(entry.Actor),
			escapeCSV(entry.IP),
			entry.Success,
			escapeCSV(entry.Error),
		))
	}

	return sb.String()
}

func (ar *AuditReporter) matchEntry(entry *AuditEntry, query *AuditQuery) bool {
	if query.Operation != "" && entry.Operation != query.Operation {
		return false
	}
	if query.Actor != "" && !strings.EqualFold(entry.Actor, query.Actor) {
		return false
	}
	if query.SecretKey != "" && entry.SecretKey != query.SecretKey {
		return false
	}
	if query.SnapshotID != "" && entry.SnapshotID != query.SnapshotID {
		return false
	}
	if query.LockID != "" && entry.LockID != query.LockID {
		return false
	}
	if !query.StartTime.IsZero() && entry.Timestamp.Before(query.StartTime) {
		return false
	}
	if !query.EndTime.IsZero() && entry.Timestamp.After(query.EndTime) {
		return false
	}
	if query.Success != nil && entry.Success != *query.Success {
		return false
	}
	return true
}

func escapeCSV(value string) string {
	if strings.ContainsAny(value, `,"`) {
		return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
	}
	return value
}
