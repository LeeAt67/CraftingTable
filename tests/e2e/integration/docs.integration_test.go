package integration

import (
	"net/http"
	"net/url"
	"testing"
)

type docNodeSummaryResponse struct {
	ID        string  `json:"id"`
	ParentID  *string `json:"parentId"`
	Kind      string  `json:"kind"`
	Title     string  `json:"title"`
	SortOrder int     `json:"sortOrder"`
	CreatedAt string  `json:"createdAt"`
	UpdatedAt string  `json:"updatedAt"`
}

type docNodeResponse struct {
	docNodeSummaryResponse
	Content string `json:"content"`
}

type docSearchHitResponse struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	ParentID  *string  `json:"parentId"`
	Path      []string `json:"path"`
	UpdatedAt string   `json:"updatedAt"`
}

type docSearchResponse struct {
	Items      []docSearchHitResponse `json:"items"`
	NextCursor *string                `json:"nextCursor"`
}

type docNodeEnvelope struct {
	Data docNodeResponse `json:"data"`
}

type docTreeEnvelope struct {
	Data []docNodeSummaryResponse `json:"data"`
}

type docSearchEnvelope struct {
	Data docSearchResponse `json:"data"`
}

type docSaveEnvelope struct {
	Data struct {
		ID        string `json:"id"`
		UpdatedAt string `json:"updatedAt"`
	} `json:"data"`
}

type docDeleteEnvelope struct {
	Data struct {
		DeletedIDs []string `json:"deletedIds"`
	} `json:"data"`
}

func TestDocsAPIEditsFolderAndDocument(t *testing.T) {
	server := startServer(t, "")

	status, folder := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"folder","title":"工作","clientRequestId":"req-folder"}`,
	)
	if status != http.StatusOK || folder.Data.Kind != "folder" {
		t.Fatalf("create folder = %d/%#v", status, folder.Data)
	}

	status, document := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"document","title":"接口设计","parentId":"`+folder.Data.ID+`","clientRequestId":"req-doc"}`,
	)
	if status != http.StatusOK || document.Data.ParentID == nil {
		t.Fatalf("create document = %d/%#v", status, document.Data)
	}
	if document.Data.Content != "" {
		t.Fatalf("new document content = %q, want empty", document.Data.Content)
	}

	status, duplicate := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"document","title":"重复请求","clientRequestId":"req-doc"}`,
	)
	if status != http.StatusOK || duplicate.Data.ID != document.Data.ID {
		t.Fatalf("idempotent create = %d/%#v", status, duplicate.Data)
	}

	status, saved := requestJSON[docSaveEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/save"),
		`{"id":"`+document.Data.ID+`","content":"# 接口设计\n\n- 列表\n- 详情"}`,
	)
	if status != http.StatusOK || saved.Data.UpdatedAt == "" {
		t.Fatalf("save = %d/%#v", status, saved.Data)
	}

	status, loaded := requestJSON[docNodeEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/item?id="+document.Data.ID),
		"",
	)
	if status != http.StatusOK || loaded.Data.Content != "# 接口设计\n\n- 列表\n- 详情" {
		t.Fatalf("load = %d/%#v", status, loaded.Data)
	}

	status, tree := requestJSON[docTreeEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/tree"),
		"",
	)
	if status != http.StatusOK || len(tree.Data) != 2 {
		t.Fatalf("tree = %d/%#v", status, tree.Data)
	}
	if tree.Data[0].Kind != "folder" {
		t.Fatalf("tree order = %#v", tree.Data)
	}

	status, renamed := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/rename"),
		`{"id":"`+document.Data.ID+`","title":"  改名后的文档  "}`,
	)
	if status != http.StatusOK || renamed.Data.Title != "改名后的文档" {
		t.Fatalf("rename = %d/%#v", status, renamed.Data)
	}

	status, moved := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/move"),
		`{"id":"`+document.Data.ID+`","parentId":null}`,
	)
	if status != http.StatusOK || moved.Data.ParentID != nil {
		t.Fatalf("move = %d/%#v", status, moved.Data)
	}
}

func TestDocsAPIValidatesInput(t *testing.T) {
	server := startServer(t, "")

	cases := []struct {
		name    string
		path    string
		body    string
		message string
	}{
		{
			name: "invalid kind",
			path: "/api/v1/docs/create",
			body: `{"kind":"page","title":"错类型"}`,
		},
		{
			name: "blank parent move",
			path: "/api/v1/docs/move",
			body: `{"id":"missing","parentId":null}`,
		},
	}

	for _, test := range cases {
		status, failure := requestJSON[errorEnvelope](
			t,
			http.MethodPost,
			server.URL(test.path),
			test.body,
		)
		if test.name == "invalid kind" {
			if status != http.StatusBadRequest || failure.Error.Code != "VALIDATION_ERROR" {
				t.Fatalf("%s = %d/%#v", test.name, status, failure.Error)
			}
			continue
		}
		if status != http.StatusNotFound || failure.Error.Code != "NOT_FOUND" {
			t.Fatalf("%s = %d/%#v", test.name, status, failure.Error)
		}
	}

	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/save"),
		`{"id":"missing","content":"正文"}`,
	)
	if status != http.StatusNotFound || failure.Error.Code != "NOT_FOUND" {
		t.Fatalf("save missing = %d/%#v", status, failure.Error)
	}
}

func TestDocsAPISearchesAndPaginates(t *testing.T) {
	server := startServer(t, "")

	status, folder := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"folder","title":"知识库"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create folder = %d", status)
	}

	for _, title := range []string{"接口一", "接口二", "接口三"} {
		status, created := requestJSON[docNodeEnvelope](
			t,
			http.MethodPost,
			server.URL("/api/v1/docs/create"),
			`{"kind":"document","title":"`+title+`","parentId":"`+folder.Data.ID+`"}`,
		)
		if status != http.StatusOK {
			t.Fatalf("create %q = %d", title, status)
		}
		_ = created
	}

	status, first := requestJSON[docSearchEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/search?q="+url.QueryEscape("接口")+"&limit=2"),
		"",
	)
	if status != http.StatusOK || len(first.Data.Items) != 2 {
		t.Fatalf("first page = %d/%#v", status, first.Data)
	}
	if first.Data.NextCursor == nil {
		t.Fatal("next cursor is missing")
	}
	if len(first.Data.Items[0].Path) != 1 || first.Data.Items[0].Path[0] != "知识库" {
		t.Fatalf("path = %#v", first.Data.Items[0].Path)
	}

	status, second := requestJSON[docSearchEnvelope](
		t,
		http.MethodGet,
		server.URL(
			"/api/v1/docs/search?q="+url.QueryEscape("接口")+
				"&limit=2&cursor="+url.QueryEscape(*first.Data.NextCursor),
		),
		"",
	)
	if status != http.StatusOK || len(second.Data.Items) != 1 {
		t.Fatalf("second page = %d/%#v", status, second.Data)
	}

	status, failure := requestJSON[errorEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/search?q="+url.QueryEscape("接口")+"&cursor=broken"),
		"",
	)
	if status != http.StatusBadRequest || failure.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("invalid cursor = %d/%#v", status, failure.Error)
	}

	status, empty := requestJSON[docSearchEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/search?q="),
		"",
	)
	if status != http.StatusOK || len(empty.Data.Items) != 0 {
		t.Fatalf("empty query = %d/%#v", status, empty.Data)
	}
}

func TestDocsAPIDeletesSubtree(t *testing.T) {
	server := startServer(t, "")

	status, folder := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"folder","title":"待删除"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create folder = %d", status)
	}
	status, document := requestJSON[docNodeEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/create"),
		`{"kind":"document","title":"子文档","parentId":"`+folder.Data.ID+`"}`,
	)
	if status != http.StatusOK {
		t.Fatalf("create document = %d", status)
	}

	status, deleted := requestJSON[docDeleteEnvelope](
		t,
		http.MethodPost,
		server.URL("/api/v1/docs/delete"),
		`{"id":"`+folder.Data.ID+`"}`,
	)
	if status != http.StatusOK || len(deleted.Data.DeletedIDs) != 2 {
		t.Fatalf("delete = %d/%#v", status, deleted.Data)
	}

	status, tree := requestJSON[docTreeEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/tree"),
		"",
	)
	if status != http.StatusOK || len(tree.Data) != 0 {
		t.Fatalf("tree after delete = %d/%#v", status, tree.Data)
	}

	status, _ = requestJSON[errorEnvelope](
		t,
		http.MethodGet,
		server.URL("/api/v1/docs/item?id="+document.Data.ID),
		"",
	)
	if status != http.StatusNotFound {
		t.Fatalf("deleted item status = %d, want 404", status)
	}
}
