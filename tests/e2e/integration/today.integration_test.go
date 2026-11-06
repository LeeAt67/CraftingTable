package integration

import (
	"net/http"
	"testing"
	"time"
)

type categoryResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type itemResponse struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Status      string  `json:"status"`
	CategoryID  *string `json:"categoryId"`
	StartAt     *string `json:"startAt"`
	EndAt       *string `json:"endAt"`
	CompletedAt *string `json:"completedAt"`
	CreatedAt   string  `json:"createdAt"`
	UpdatedAt   string  `json:"updatedAt"`
}

type apiError struct {
	Code      string         `json:"code"`
	Kind      string         `json:"kind"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	RequestID string         `json:"requestId"`
	Details   map[string]any `json:"details"`
}

type categoriesEnvelope struct {
	Data []categoryResponse `json:"data"`
}

type itemEnvelope struct {
	Data itemResponse `json:"data"`
}

type itemsEnvelope struct {
	Data []itemResponse `json:"data"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

func TestTodayAPIRoundTrip(t *testing.T) {
	server := startServer(t, "")

	status, categories := requestJSON[categoriesEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/categories"),
		"",
	)
	if status != http.StatusOK {
		t.Fatalf("categories status = %d, want 200", status)
	}
	if len(categories.Data) != 3 {
		t.Fatalf("category count = %d, want 3", len(categories.Data))
	}

	status, created := requestJSON[itemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/create"),
		`{"title":"整理 Go API","categoryId":"work"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create status = %d, want 200", status)
	}
	if created.Data.Status != "unplanned" || created.Data.Kind != "task" {
		t.Fatalf("created item = %#v", created.Data)
	}
	if created.Data.CategoryID == nil || *created.Data.CategoryID != "work" {
		t.Fatalf("categoryId = %#v, want work", created.Data.CategoryID)
	}

	status, today := requestJSON[itemsEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/today"),
		"",
	)
	if status != http.StatusOK || len(today.Data) != 1 {
		t.Fatalf("today status/count = %d/%d, want 200/1", status, len(today.Data))
	}

	status, completed := requestJSON[itemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/complete"),
		`{"id":"`+created.Data.ID+`"}`,
	)
	if status != http.StatusOK || completed.Data.Status != "done" {
		t.Fatalf("completed status/item = %d/%#v", status, completed.Data)
	}
	if completed.Data.CompletedAt == nil {
		t.Fatal("completedAt is missing")
	}

	status, restored := requestJSON[itemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/uncomplete"),
		`{"id":"`+created.Data.ID+`"}`,
	)
	if status != http.StatusOK || restored.Data.Status != "unplanned" {
		t.Fatalf("restored status/item = %d/%#v", status, restored.Data)
	}
}

func TestTodayAPIUpdatesPlannedItemAndValidatesInput(t *testing.T) {
	server := startServer(t, "")
	startAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)

	status, planned := requestJSON[itemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/create"),
		`{"title":"规划任务","startAt":"`+startAt+`"}`,
	)
	if status != http.StatusOK || planned.Data.Status != "planned" {
		t.Fatalf("planned status/item = %d/%#v", status, planned.Data)
	}

	status, updated := requestJSON[itemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/update"),
		`{"id":"`+planned.Data.ID+`","title":"更新后的任务","description":"Go 服务"}`,
	)
	if status != http.StatusOK ||
		updated.Data.Title != "更新后的任务" ||
		updated.Data.Description == nil ||
		*updated.Data.Description != "Go 服务" {
		t.Fatalf("updated status/item = %d/%#v", status, updated.Data)
	}

	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/create"),
		`{"title":"  "}`,
	)
	if status != http.StatusBadRequest {
		t.Fatalf("validation status = %d, want 400", status)
	}
	if failure.Error.Code != "VALIDATION_ERROR" ||
		failure.Error.Kind != "validation" ||
		failure.Error.RequestID == "" {
		t.Fatalf("validation error = %#v", failure.Error)
	}
}

type deletedItemEnvelope struct {
	Data struct {
		ID        string `json:"id"`
		DeletedAt string `json:"deletedAt"`
	} `json:"data"`
}

func createTask(t *testing.T, server *testServer, body string) itemResponse {
	t.Helper()
	status, created := requestJSON[itemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/create"),
		body,
	)
	if status != http.StatusOK {
		t.Fatalf("create status = %d, body = %s", status, body)
	}
	return created.Data
}

func listTitles(t *testing.T, server *testServer, path string) []string {
	t.Helper()
	status, items := requestJSON[itemsEnvelope](
		t,
		http.MethodGet,
		server.URL(path),
		"",
	)
	if status != http.StatusOK {
		t.Fatalf("list %s status = %d", path, status)
	}
	titles := make([]string, 0, len(items.Data))
	for _, item := range items.Data {
		titles = append(titles, item.Title)
	}
	return titles
}

func TestTodayAPIFiltersByLocalDayAndTimezone(t *testing.T) {
	server := startServer(t, "")

	cases := []string{
		"2026-11-04T15:59:00Z", // 11-04 23:59 Asia/Shanghai，前一天
		"2026-11-04T16:01:00Z", // 11-05 00:01，当天开始
		"2026-11-05T15:59:00Z", // 11-05 23:59，当天结束
		"2026-11-05T16:01:00Z", // 11-06 00:01，后一天
	}
	for _, startAt := range cases {
		createTask(t, server, `{"title":"`+startAt+`","startAt":"`+startAt+`"}`)
	}

	titles := listTitles(t, server, "/api/v1/today?date=2026-11-05&tz=Asia/Shanghai")
	if len(titles) != 2 {
		t.Fatalf("titles = %v, want 2 items", titles)
	}
	if titles[0] != "2026-11-04T16:01:00Z" || titles[1] != "2026-11-05T15:59:00Z" {
		t.Fatalf("titles = %v", titles)
	}
}

func TestTodayAPIDefaultsToServerLocalDay(t *testing.T) {
	server := startServer(t, "")
	now := time.Now().UTC().Format(time.RFC3339)
	createTask(t, server, `{"title":"本地当天","startAt":"`+now+`"}`)

	if titles := listTitles(t, server, "/api/v1/today"); len(titles) != 1 {
		t.Fatalf("titles = %v, want the item created now", titles)
	}
}

func TestTodayAPIRejectsInvalidDateAndTimeZone(t *testing.T) {
	server := startServer(t, "")

	for _, path := range []string{
		"/api/v1/today?date=2026-13-01",
		"/api/v1/today?date=2026-11-05&tz=Mars/Olympus",
	} {
		status, failure := requestJSON[errorEnvelope](
			t,
			http.MethodGet,
			server.URL(path),
			"",
		)
		if status != http.StatusBadRequest ||
			failure.Error.Code != "VALIDATION_ERROR" {
			t.Fatalf("%s = %d/%#v", path, status, failure.Error)
		}
	}
}

func TestTodayAPIHandlesDaylightSavingDay(t *testing.T) {
	server := startServer(t, "")

	for _, startAt := range []string{
		"2026-03-08T05:30:00Z", // 03-08 00:30 EST
		"2026-03-09T03:30:00Z", // 03-08 23:30 EDT
		"2026-03-09T04:30:00Z", // 03-09 00:30 EDT
	} {
		createTask(t, server, `{"title":"`+startAt+`","startAt":"`+startAt+`"}`)
	}

	titles := listTitles(
		t,
		server,
		"/api/v1/today?date=2026-03-08&tz=America/New_York",
	)
	if len(titles) != 2 {
		t.Fatalf("titles = %v, want the 23 hour day", titles)
	}
	for _, title := range titles {
		if title == "2026-03-09T04:30:00Z" {
			t.Fatalf("titles = %v, next day leaked in", titles)
		}
	}
}

func TestTodayAPIValidatesTimeRangeAndNormalizesOffset(t *testing.T) {
	server := startServer(t, "")

	for _, body := range []string{
		`{"title":"倒序","startAt":"2026-11-05T10:00:00Z","endAt":"2026-11-05T09:00:00Z"}`,
		`{"title":"相等","startAt":"2026-11-05T09:00:00Z","endAt":"2026-11-05T09:00:00Z"}`,
		`{"title":"缺开始","endAt":"2026-11-05T10:00:00Z"}`,
	} {
		status, failure := requestJSON[errorEnvelope](
			t,
			http.MethodPost,
			server.URL("/api/v1/item/create"),
			body,
		)
		if status != http.StatusBadRequest ||
			failure.Error.Code != "VALIDATION_ERROR" {
			t.Fatalf("%s = %d/%#v", body, status, failure.Error)
		}
	}

	created := createTask(
		t,
		server,
		`{"title":"偏移量","startAt":"2026-11-05T09:30:00+08:00"}`,
	)
	if created.StartAt == nil || *created.StartAt != "2026-11-05T01:30:00.000Z" {
		t.Fatalf("startAt = %v", created.StartAt)
	}
	if created.CreatedAt != "2026-11-05T01:30:00.000Z" && len(created.CreatedAt) != len("2026-11-05T01:30:00.000Z") {
		t.Fatalf("createdAt = %q, want fixed width", created.CreatedAt)
	}
}

func TestTodayAPIDeleteIsIdempotentAndHidesItem(t *testing.T) {
	server := startServer(t, "")
	created := createTask(t, server, `{"title":"待删除"}`)

	status, first := requestJSON[deletedItemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/delete"),
		`{"id":"`+created.ID+`"}`,
	)
	if status != http.StatusOK || first.Data.DeletedAt == "" {
		t.Fatalf("delete = %d/%#v", status, first.Data)
	}

	status, second := requestJSON[deletedItemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/delete"),
		`{"id":"`+created.ID+`"}`,
	)
	if status != http.StatusOK || second.Data.DeletedAt != first.Data.DeletedAt {
		t.Fatalf("second delete = %d/%#v, want %q", status, second.Data, first.Data.DeletedAt)
	}

	if titles := listTitles(t, server, "/api/v1/today"); len(titles) != 0 {
		t.Fatalf("titles after delete = %v", titles)
	}

	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/delete"),
		`{"id":"missing-item"}`,
	)
	if status != http.StatusNotFound || failure.Error.Code != "NOT_FOUND" {
		t.Fatalf("delete missing = %d/%#v", status, failure.Error)
	}
}

func TestTodayAPIDeletedItemRejectsWritesAndCanBeRestored(t *testing.T) {
	server := startServer(t, "")
	created := createTask(
		t,
		server,
		`{"title":"可恢复","categoryId":"work","startAt":"2026-11-05T09:00:00Z"}`,
	)
	if created.Status != "planned" {
		t.Fatalf("status = %q", created.Status)
	}

	if status, _ := requestJSON[deletedItemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/delete"),
		`{"id":"`+created.ID+`"}`,
	); status != http.StatusOK {
		t.Fatalf("delete status = %d", status)
	}

	for _, path := range []string{
		"/api/v1/item/complete",
		"/api/v1/item/uncomplete",
	} {
		status, failure := requestJSON[errorEnvelope](
			t,
			http.MethodPost,
			server.URL(path),
			`{"id":"`+created.ID+`"}`,
		)
		if status != http.StatusNotFound {
			t.Fatalf("%s = %d/%#v", path, status, failure.Error)
		}
	}

	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/update"),
		`{"id":"`+created.ID+`","title":"改名"}`,
	)
	if status != http.StatusNotFound {
		t.Fatalf("update deleted = %d/%#v", status, failure.Error)
	}

	status, restored := requestJSON[itemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/restore"),
		`{"id":"`+created.ID+`"}`,
	)
	if status != http.StatusOK ||
		restored.Data.Status != "planned" ||
		restored.Data.CategoryID == nil ||
		*restored.Data.CategoryID != "work" {
		t.Fatalf("restore = %d/%#v", status, restored.Data)
	}

	status, again := requestJSON[itemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/restore"),
		`{"id":"`+created.ID+`"}`,
	)
	if status != http.StatusOK || again.Data.ID != created.ID {
		t.Fatalf("idempotent restore = %d/%#v", status, again.Data)
	}

	status, failure = requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/restore"),
		`{"id":"missing-item"}`,
	)
	if status != http.StatusNotFound {
		t.Fatalf("restore missing = %d/%#v", status, failure.Error)
	}
}

func TestTodayAPIUpdateKeepsDoneStatusAndClearsSchedule(t *testing.T) {
	server := startServer(t, "")
	planned := createTask(
		t,
		server,
		`{"title":"改期","startAt":"2026-11-05T09:00:00Z"}`,
	)
	if status, _ := requestJSON[itemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/complete"),
		`{"id":"`+planned.ID+`"}`,
	); status != http.StatusOK {
		t.Fatalf("complete status = %d", status)
	}

	status, updated := requestJSON[itemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/update"),
		`{"id":"`+planned.ID+`","startAt":"2026-11-05T11:00:00Z"}`,
	)
	if status != http.StatusOK || updated.Data.Status != "done" {
		t.Fatalf("update done = %d/%#v", status, updated.Data)
	}

	pending := createTask(
		t,
		server,
		`{"title":"清空安排","startAt":"2026-11-05T09:00:00Z"}`,
	)
	status, cleared := requestJSON[itemEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/item/update"),
		`{"id":"`+pending.ID+`","startAt":"","endAt":""}`,
	)
	if status != http.StatusOK ||
		cleared.Data.Status != "unplanned" ||
		cleared.Data.StartAt != nil ||
		cleared.Data.EndAt != nil {
		t.Fatalf("cleared = %d/%#v", status, cleared.Data)
	}
}
