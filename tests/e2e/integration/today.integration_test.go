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
	Code      string `json:"code"`
	Kind      string `json:"kind"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	RequestID string `json:"requestId"`
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
