package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestQueryAggregate(t *testing.T) {
	svc := newTestService(t)

	insertTestLogs(t, svc.db, []trafficLog{
		{Timestamp: 1000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Upload: 100, Download: 200},
		{Timestamp: 1500, SourceIP: "192.168.1.2", Host: "b.com", Process: "chrome", Outbound: "NodeA", Upload: 50, Download: 20},
		{Timestamp: 2000, SourceIP: "192.168.1.3", Host: "a.com", Process: "curl", Outbound: "DIRECT", Upload: 10, Download: 30},
	})

	got, err := svc.queryAggregate("sourceIP", 500, 3000)
	if err != nil {
		t.Fatalf("queryAggregate: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
	if got[0].Label != "192.168.1.2" || got[0].Upload != 150 || got[0].Download != 220 || got[0].Total != 370 {
		t.Fatalf("unexpected first row: %+v", got[0])
	}
}

func TestQueryTrendFillsEmptyBuckets(t *testing.T) {
	svc := newTestService(t)

	insertTestLogs(t, svc.db, []trafficLog{
		{Timestamp: 1000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Upload: 100, Download: 200},
		{Timestamp: 4100, SourceIP: "192.168.1.2", Host: "b.com", Process: "chrome", Outbound: "NodeA", Upload: 50, Download: 20},
	})

	got, err := svc.queryTrend(1000, 7000, 2000)
	if err != nil {
		t.Fatalf("queryTrend: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("expected 4 buckets, got %d", len(got))
	}
	if got[1].Timestamp != 2000 || got[1].Upload != 0 || got[1].Download != 0 {
		t.Fatalf("expected empty middle bucket, got %+v", got[1])
	}
	if got[2].Timestamp != 4000 || got[2].Upload != 50 || got[2].Download != 20 {
		t.Fatalf("unexpected populated bucket: %+v", got[2])
	}
}

func TestOpenDatabaseAddsDetailColumns(t *testing.T) {
	svc := newTestService(t)

	rows, err := svc.db.Query(`PRAGMA table_info(traffic_logs)`)
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()

	columns := map[string]bool{}
	for rows.Next() {
		var (
			cid        int
			name       string
			typeName   string
			notNull    int
			defaultV   any
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &typeName, &notNull, &defaultV, &primaryKey); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		columns[name] = true
	}

	for _, name := range []string{"destination_ip", "chains"} {
		if !columns[name] {
			t.Fatalf("expected column %q to exist", name)
		}
	}
}

func TestProcessConnectionsStoresDetailFields(t *testing.T) {
	svc := newTestService(t)

	payload := &connectionsResponse{
		Connections: []connection{
			{
				ID:       "conn-1",
				Upload:   128,
				Download: 256,
				Chains:   []string{"ProxyA", "RelayB"},
				Metadata: struct {
					SourceIP      string "json:\"sourceIP\""
					Host          string "json:\"host\""
					DestinationIP string "json:\"destinationIP\""
					Process       string "json:\"process\""
				}{
					SourceIP:      "192.168.1.8",
					Host:          "api.example.com",
					DestinationIP: "1.1.1.1",
					Process:       "curl",
				},
			},
		},
	}

	if err := svc.processConnections(payload); err != nil {
		t.Fatalf("processConnections: %v", err)
	}

	var (
		host          string
		destinationIP string
		outbound      string
		chainsRaw     string
	)
	err := svc.db.QueryRow(`
		SELECT host, destination_ip, outbound, chains
		FROM traffic_logs
		LIMIT 1
	`).Scan(&host, &destinationIP, &outbound, &chainsRaw)
	if err != nil {
		t.Fatalf("query stored log: %v", err)
	}

	if host != "api.example.com" {
		t.Fatalf("unexpected host: %q", host)
	}
	if destinationIP != "1.1.1.1" {
		t.Fatalf("unexpected destination_ip: %q", destinationIP)
	}
	if outbound != "ProxyA" {
		t.Fatalf("unexpected outbound: %q", outbound)
	}

	var chains []string
	if err := json.Unmarshal([]byte(chainsRaw), &chains); err != nil {
		t.Fatalf("unmarshal chains: %v", err)
	}
	if len(chains) != 2 || chains[0] != "ProxyA" || chains[1] != "RelayB" {
		t.Fatalf("unexpected chains: %#v", chains)
	}
}

func TestQueryAggregateLongRangeIncludesRawBoundaryData(t *testing.T) {
	svc := newTestService(t)

	insertTestLogs(t, svc.db, []trafficLog{
		{Timestamp: 30_000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Upload: 10, Download: 20},
		{Timestamp: 60_000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Upload: 30, Download: 40},
		{Timestamp: 120_000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Upload: 50, Download: 60},
		{Timestamp: 3_660_000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Upload: 70, Download: 80},
	})

	insertTestAggregates(t, svc.db, []aggregatedEntry{
		{
			BucketStart: 60_000,
			BucketEnd:   120_000,
			SourceIP:    "192.168.1.2",
			Host:        "a.com",
			Process:     "chrome",
			Outbound:    "NodeA",
			Chains:      `["DIRECT"]`,
			Upload:      30,
			Download:    40,
			Count:       1,
		},
		{
			BucketStart: 120_000,
			BucketEnd:   180_000,
			SourceIP:    "192.168.1.2",
			Host:        "a.com",
			Process:     "chrome",
			Outbound:    "NodeA",
			Chains:      `["DIRECT"]`,
			Upload:      50,
			Download:    60,
			Count:       1,
		},
	})

	got, err := svc.queryAggregate("sourceIP", 30_000, 3_660_000)
	if err != nil {
		t.Fatalf("queryAggregate: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 row, got %d", len(got))
	}
	if got[0].Upload != 160 || got[0].Download != 200 || got[0].Total != 360 || got[0].Count != 4 {
		t.Fatalf("unexpected aggregated result: %+v", got[0])
	}
}

func TestQueryTrendRebucketsAggregatedMinuteData(t *testing.T) {
	svc := newTestService(t)

	insertTestAggregates(t, svc.db, []aggregatedEntry{
		{BucketStart: 60_000, BucketEnd: 120_000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Chains: `["DIRECT"]`, Upload: 10, Download: 20, Count: 1},
		{BucketStart: 120_000, BucketEnd: 180_000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Chains: `["DIRECT"]`, Upload: 30, Download: 40, Count: 1},
		{BucketStart: 180_000, BucketEnd: 240_000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Chains: `["DIRECT"]`, Upload: 50, Download: 60, Count: 1},
		{BucketStart: 240_000, BucketEnd: 300_000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Chains: `["DIRECT"]`, Upload: 70, Download: 80, Count: 1},
	})

	got, err := svc.queryTrend(0, 600_000, 300_000)
	if err != nil {
		t.Fatalf("queryTrend: %v", err)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 buckets, got %d", len(got))
	}
	if got[0].Timestamp != 0 || got[0].Upload != 160 || got[0].Download != 200 {
		t.Fatalf("unexpected first bucket: %+v", got[0])
	}
	if got[1].Timestamp != 300_000 || got[1].Upload != 0 || got[1].Download != 0 {
		t.Fatalf("unexpected second bucket: %+v", got[1])
	}
}

func TestHandleLogsClearsAggregatesAndBuffer(t *testing.T) {
	svc := newTestService(t)

	insertTestLogs(t, svc.db, []trafficLog{
		{Timestamp: 1000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Upload: 100, Download: 200},
	})
	insertTestAggregates(t, svc.db, []aggregatedEntry{
		{BucketStart: 0, BucketEnd: 60_000, SourceIP: "192.168.1.2", Host: "a.com", Process: "chrome", Outbound: "NodeA", Chains: `["DIRECT"]`, Upload: 100, Download: 200, Count: 1},
	})
	svc.aggregateBuffer["pending"] = &aggregatedEntry{BucketStart: 60_000, BucketEnd: 120_000, SourceIP: "192.168.1.2"}

	req := httptest.NewRequest(http.MethodDelete, "/api/traffic/logs", nil)
	rec := httptest.NewRecorder()

	svc.handleLogs(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var rawCount int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM traffic_logs`).Scan(&rawCount); err != nil {
		t.Fatalf("count traffic_logs: %v", err)
	}
	if rawCount != 0 {
		t.Fatalf("expected traffic_logs to be empty, got %d rows", rawCount)
	}

	var aggregateCount int
	if err := svc.db.QueryRow(`SELECT COUNT(*) FROM traffic_aggregated`).Scan(&aggregateCount); err != nil {
		t.Fatalf("count traffic_aggregated: %v", err)
	}
	if aggregateCount != 0 {
		t.Fatalf("expected traffic_aggregated to be empty, got %d rows", aggregateCount)
	}

	if len(svc.aggregateBuffer) != 0 {
		t.Fatalf("expected aggregate buffer to be empty, got %d entries", len(svc.aggregateBuffer))
	}
}

func TestHandleConnectionDetailsReturnsGroupedDetails(t *testing.T) {
	svc := newTestService(t)

	insertTestLogs(t, svc.db, []trafficLog{
		{
			Timestamp:     1000,
			SourceIP:      "192.168.1.2",
			Host:          "a.com",
			DestinationIP: "1.1.1.1",
			Process:       "chrome",
			Outbound:      "NodeA",
			Chains:        []string{"NodeA", "RelayA"},
			Upload:        100,
			Download:      200,
		},
		{
			Timestamp:     1100,
			SourceIP:      "192.168.1.2",
			Host:          "a.com",
			DestinationIP: "1.1.1.1",
			Process:       "chrome",
			Outbound:      "NodeA",
			Chains:        []string{"NodeA", "RelayA"},
			Upload:        10,
			Download:      20,
		},
		{
			Timestamp:     1200,
			SourceIP:      "192.168.1.3",
			Host:          "a.com",
			DestinationIP: "8.8.8.8",
			Process:       "curl",
			Outbound:      "NodeB",
			Chains:        []string{"NodeB"},
			Upload:        30,
			Download:      40,
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/traffic/details?dimension=host&primary=a.com&secondary=192.168.1.2&start=500&end=2000", nil)
	rec := httptest.NewRecorder()

	svc.handleConnectionDetails(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
	}

	var got []connectionDetail
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 grouped detail, got %d", len(got))
	}
	if got[0].DestinationIP != "1.1.1.1" {
		t.Fatalf("unexpected destination ip: %+v", got[0])
	}
	if got[0].SourceIP != "192.168.1.2" {
		t.Fatalf("unexpected source ip: %+v", got[0])
	}
	if got[0].Process != "chrome" || got[0].Outbound != "NodeA" {
		t.Fatalf("unexpected routing info: %+v", got[0])
	}
	if got[0].Upload != 110 || got[0].Download != 220 || got[0].Count != 2 {
		t.Fatalf("unexpected aggregates: %+v", got[0])
	}
	if len(got[0].Chains) != 2 || got[0].Chains[0] != "NodeA" {
		t.Fatalf("unexpected chains: %+v", got[0])
	}
}

func TestEmbeddedIndexDisablesPeriodicAutoRefresh(t *testing.T) {
	content, err := webAssets.ReadFile("web/index.html")
	if err != nil {
		t.Fatalf("read embedded index.html: %v", err)
	}

	html := string(content)
	if !strings.Contains(html, `elements.refreshBtn.addEventListener("click", loadData)`) {
		t.Fatalf("expected manual refresh handler to remain available")
	}
	if strings.Contains(html, "setInterval(loadData, 30000)") {
		t.Fatalf("expected periodic auto refresh to be removed")
	}
	if !strings.Contains(html, "updateCustomInputs()\n      updateViewHints()\n      loadData()") {
		t.Fatalf("expected initial page load to fetch data once")
	}
	for _, label := range []string{"1 天", "7 天", "15 天", "30 天", "自定义"} {
		if !strings.Contains(html, label) {
			t.Fatalf("expected range option %q to exist", label)
		}
	}
	for _, label := range []string{"最近 1 小时", "最近 24 小时"} {
		if strings.Contains(html, label) {
			t.Fatalf("expected old range option %q to be removed", label)
		}
	}
}

func newTestService(t *testing.T) *service {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "traffic.db")
	db, err := openDatabase(dbPath)
	if err != nil {
		t.Fatalf("openDatabase: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	return &service{
		db:              db,
		lastConnections: make(map[string]connection),
		aggregateBuffer: make(map[string]*aggregatedEntry),
	}
}

func insertTestLogs(t *testing.T, db *sql.DB, logs []trafficLog) {
	t.Helper()

	for _, entry := range logs {
		chainsRaw, err := json.Marshal(entry.Chains)
		if err != nil {
			t.Fatalf("marshal chains: %v", err)
		}

		_, err = db.Exec(
			`INSERT INTO traffic_logs (timestamp, source_ip, host, destination_ip, process, outbound, chains, upload, download)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			entry.Timestamp,
			entry.SourceIP,
			entry.Host,
			entry.DestinationIP,
			entry.Process,
			entry.Outbound,
			string(chainsRaw),
			entry.Upload,
			entry.Download,
		)
		if err != nil {
			t.Fatalf("insert log: %v", err)
		}
	}
}

func insertTestAggregates(t *testing.T, db *sql.DB, entries []aggregatedEntry) {
	t.Helper()

	for _, entry := range entries {
		_, err := db.Exec(
			`INSERT INTO traffic_aggregated
			 (bucket_start, bucket_end, source_ip, host, destination_ip, process, outbound, chains, upload, download, count)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			entry.BucketStart,
			entry.BucketEnd,
			entry.SourceIP,
			entry.Host,
			entry.DestinationIP,
			entry.Process,
			entry.Outbound,
			entry.Chains,
			entry.Upload,
			entry.Download,
			entry.Count,
		)
		if err != nil {
			t.Fatalf("insert aggregate: %v", err)
		}
	}
}
