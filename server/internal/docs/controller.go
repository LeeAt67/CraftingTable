package docs

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/leeat/craftingtable/server/internal/platform/httpapi"
)

// Controller 将文档领域用例适配为 HTTP。
type Controller struct {
	service *Service
	logger  *slog.Logger
}

// NewController 创建文档 HTTP Controller。
func NewController(repository Repository, logger *slog.Logger) *Controller {
	return &Controller{
		service: NewService(repository),
		logger:  logger,
	}
}

// Register 注册文档模块的全部 HTTP 路由。
func (c *Controller) Register(router *httpapi.Router) {
	router.Handle("GET /api/v1/docs/tree", c.listTree)
	router.Handle("GET /api/v1/docs/item", c.getNode)
	router.Handle("GET /api/v1/docs/search", c.search)
	router.Handle("POST /api/v1/docs/create", c.createNode)
	router.Handle("POST /api/v1/docs/rename", c.renameNode)
	router.Handle("POST /api/v1/docs/move", c.moveNode)
	router.Handle("POST /api/v1/docs/save", c.saveContent)
	router.Handle("POST /api/v1/docs/delete", c.deleteNode)
}

type createNodeRequest struct {
	Kind            string  `json:"kind"`
	ParentID        *string `json:"parentId"`
	Title           string  `json:"title"`
	ClientRequestID *string `json:"clientRequestId"`
}

type renameNodeRequest struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type moveNodeRequest struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parentId"`
}

type saveContentRequest struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type deleteNodeRequest struct {
	ID string `json:"id"`
}

func (c *Controller) listTree(w http.ResponseWriter, r *http.Request) {
	nodes, err := c.service.ListTree(r.Context())
	if err != nil {
		c.internalError(w, r, "加载文档目录失败", err)
		return
	}
	httpapi.OK(w, http.StatusOK, nodes)
}

func (c *Controller) getNode(w http.ResponseWriter, r *http.Request) {
	node, err := c.service.GetNode(r.Context(), r.URL.Query().Get("id"))
	if err != nil {
		c.writeError(w, r, err, "加载文档失败")
		return
	}
	httpapi.OK(w, http.StatusOK, node)
}

func (c *Controller) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	limit, _ := strconv.Atoi(strings.TrimSpace(query.Get("limit")))

	page, err := c.service.Search(
		r.Context(),
		query.Get("q"),
		limit,
		query.Get("cursor"),
	)
	if err != nil {
		c.writeError(w, r, err, "搜索文档失败")
		return
	}
	httpapi.OK(w, http.StatusOK, map[string]any{
		"items":      page.Items,
		"nextCursor": page.NextCursor,
	})
}

func (c *Controller) createNode(w http.ResponseWriter, r *http.Request) {
	var request createNodeRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		c.writeError(w, r, ErrInvalidInput, "创建文档失败")
		return
	}

	node, err := c.service.CreateNode(r.Context(), CreateNodeInput{
		Kind:            NodeKind(strings.TrimSpace(request.Kind)),
		ParentID:        request.ParentID,
		Title:           request.Title,
		ClientRequestID: request.ClientRequestID,
	})
	if err != nil {
		c.writeError(w, r, err, "创建文档失败")
		return
	}
	httpapi.OK(w, http.StatusOK, node)
}

func (c *Controller) renameNode(w http.ResponseWriter, r *http.Request) {
	var request renameNodeRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		c.writeError(w, r, ErrInvalidInput, "重命名失败")
		return
	}

	node, err := c.service.RenameNode(r.Context(), RenameNodeInput{
		ID:    request.ID,
		Title: request.Title,
	})
	if err != nil {
		c.writeError(w, r, err, "重命名失败")
		return
	}
	httpapi.OK(w, http.StatusOK, node)
}

func (c *Controller) moveNode(w http.ResponseWriter, r *http.Request) {
	var request moveNodeRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		c.writeError(w, r, ErrInvalidInput, "移动失败")
		return
	}

	node, err := c.service.MoveNode(r.Context(), MoveNodeInput{
		ID:       request.ID,
		ParentID: request.ParentID,
	})
	if err != nil {
		c.writeError(w, r, err, "移动失败")
		return
	}
	httpapi.OK(w, http.StatusOK, node)
}

func (c *Controller) saveContent(w http.ResponseWriter, r *http.Request) {
	var request saveContentRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		c.writeError(w, r, ErrInvalidInput, "保存文档失败")
		return
	}

	updatedAt, err := c.service.SaveContent(r.Context(), SaveContentInput{
		ID:      request.ID,
		Content: request.Content,
	})
	if err != nil {
		c.writeError(w, r, err, "保存文档失败")
		return
	}
	httpapi.OK(w, http.StatusOK, map[string]string{
		"id":        request.ID,
		"updatedAt": updatedAt,
	})
}

func (c *Controller) deleteNode(w http.ResponseWriter, r *http.Request) {
	var request deleteNodeRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		c.writeError(w, r, ErrInvalidInput, "删除失败")
		return
	}

	identifiers, err := c.service.DeleteNode(r.Context(), request.ID)
	if err != nil {
		c.writeError(w, r, err, "删除失败")
		return
	}
	httpapi.OK(w, http.StatusOK, map[string]any{"deletedIds": identifiers})
}

func (c *Controller) writeError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
	fallback string,
) {
	switch {
	case errors.Is(err, ErrNodeNotFound):
		httpapi.Fail(w, r, http.StatusNotFound, httpapi.ErrorBody{
			Code:      "NOT_FOUND",
			Kind:      "not_found",
			Message:   "文档不存在或已被删除",
			Retryable: false,
		})
	case errors.Is(err, ErrParentNotFolder):
		c.validationError(w, r, "目标位置不是文件夹")
	case errors.Is(err, ErrInvalidMove):
		c.validationError(w, r, "不能移动到自身或子节点")
	case errors.Is(err, ErrFolderHasContent):
		c.validationError(w, r, "文件夹不能保存正文")
	case errors.Is(err, ErrContentTooLong):
		c.validationError(w, r, "文档内容过长（上限 200000 字符）")
	case errors.Is(err, ErrInvalidCursor):
		c.validationError(w, r, "搜索游标与查询词不匹配")
	case errors.Is(err, ErrInvalidInput):
		c.validationError(w, r, "参数错误")
	default:
		c.internalError(w, r, fallback, err)
	}
}

func (c *Controller) validationError(
	w http.ResponseWriter,
	r *http.Request,
	message string,
) {
	httpapi.Fail(w, r, http.StatusBadRequest, httpapi.ErrorBody{
		Code:      "VALIDATION_ERROR",
		Kind:      "validation",
		Message:   message,
		Retryable: false,
	})
}

func (c *Controller) internalError(
	w http.ResponseWriter,
	r *http.Request,
	message string,
	err error,
) {
	c.logger.Error(
		"docs request failed",
		"request_id", httpapi.RequestID(r),
		"error", err,
	)
	httpapi.Fail(w, r, http.StatusInternalServerError, httpapi.ErrorBody{
		Code:      "INTERNAL_ERROR",
		Kind:      "unknown",
		Message:   message,
		Retryable: true,
	})
}
