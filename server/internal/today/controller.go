package today

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/leeat/craftingtable/server/internal/platform/httpapi"
)

const (
	maxTitleRunes       = 200
	maxDescriptionRunes = 2000
)

// Controller 将今日领域用例适配为 HTTP。
type Controller struct {
	repository Repository
	logger     *slog.Logger
}

// NewController 创建今日 HTTP Controller。
func NewController(repository Repository, logger *slog.Logger) *Controller {
	return &Controller{repository: repository, logger: logger}
}

// Register 注册今日模块的全部 HTTP 路由。
func (c *Controller) Register(router *httpapi.Router) {
	router.Handle("GET /api/v1/today", c.listToday)
	router.Handle("GET /api/v1/categories", c.listCategories)
	router.Handle("POST /api/v1/item/create", c.createItem)
	router.Handle("POST /api/v1/item/update", c.updateItem)
	router.Handle("POST /api/v1/item/complete", c.completeItem)
	router.Handle("POST /api/v1/item/uncomplete", c.uncompleteItem)
	router.Handle("POST /api/v1/item/delete", c.deleteItem)
	router.Handle("POST /api/v1/item/restore", c.restoreItem)
}

type createItemRequest struct {
	Title      string  `json:"title"`
	CategoryID *string `json:"categoryId"`
	StartAt    *string `json:"startAt"`
	EndAt      *string `json:"endAt"`
}

type updateItemRequest struct {
	ID          string  `json:"id"`
	Title       *string `json:"title"`
	Description *string `json:"description"`
	CategoryID  *string `json:"categoryId"`
	StartAt     *string `json:"startAt"`
	EndAt       *string `json:"endAt"`
}

type itemIDRequest struct {
	ID string `json:"id"`
}

func (c *Controller) listToday(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	dayRange, message := ResolveDayRange(
		query.Get("date"),
		query.Get("tz"),
		time.Now(),
	)
	if message != "" {
		writeValidationError(w, r, message)
		return
	}

	items, err := c.repository.ListToday(r.Context(), dayRange.Start, dayRange.End)
	if err != nil {
		c.internalError(w, r, "加载今日事务失败", err)
		return
	}
	httpapi.OK(w, http.StatusOK, items)
}

func (c *Controller) listCategories(w http.ResponseWriter, r *http.Request) {
	categories, err := c.repository.ListCategories(r.Context())
	if err != nil {
		c.internalError(w, r, "加载分类失败", err)
		return
	}
	httpapi.OK(w, http.StatusOK, categories)
}

func (c *Controller) createItem(w http.ResponseWriter, r *http.Request) {
	var request createItemRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		writeValidationError(w, r, "参数错误")
		return
	}

	title, message := validateTitle(request.Title)
	if message != "" {
		writeValidationError(w, r, message)
		return
	}
	if request.CategoryID != nil && strings.TrimSpace(*request.CategoryID) == "" {
		writeValidationError(w, r, "分类不能为空")
		return
	}
	startAt, message := NormalizeTimestamp(request.StartAt)
	if message != "" {
		writeValidationError(w, r, "startAt "+message)
		return
	}
	endAt, message := NormalizeTimestamp(request.EndAt)
	if message != "" {
		writeValidationError(w, r, "endAt "+message)
		return
	}
	if problem := TimeRangeProblem(startAt, endAt); problem != "" {
		writeValidationError(w, r, problem)
		return
	}

	item, err := c.repository.CreateItem(r.Context(), CreateItemInput{
		Title:      title,
		CategoryID: request.CategoryID,
		StartAt:    startAt,
		EndAt:      endAt,
	})
	if err != nil {
		c.writeRepositoryError(w, r, err)
		return
	}
	httpapi.OK(w, http.StatusOK, item)
}

func (c *Controller) updateItem(w http.ResponseWriter, r *http.Request) {
	var request updateItemRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil {
		writeValidationError(w, r, "参数错误")
		return
	}
	if strings.TrimSpace(request.ID) == "" {
		writeValidationError(w, r, "缺少 id")
		return
	}

	if request.Title != nil {
		title, message := validateTitle(*request.Title)
		if message != "" {
			writeValidationError(w, r, message)
			return
		}
		request.Title = &title
	}
	if request.Description != nil &&
		utf8.RuneCountInString(*request.Description) > maxDescriptionRunes {
		writeValidationError(w, r, "描述不能超过 2000 个字符")
		return
	}
	if request.CategoryID != nil && strings.TrimSpace(*request.CategoryID) == "" {
		writeValidationError(w, r, "分类不能为空")
		return
	}
	startAt, message := NormalizeOptionalTimestamp(request.StartAt)
	if message != "" {
		writeValidationError(w, r, "startAt "+message)
		return
	}
	endAt, message := NormalizeOptionalTimestamp(request.EndAt)
	if message != "" {
		writeValidationError(w, r, "endAt "+message)
		return
	}

	item, err := c.repository.UpdateItem(r.Context(), UpdateItemInput{
		ID:          strings.TrimSpace(request.ID),
		Title:       request.Title,
		Description: request.Description,
		CategoryID:  request.CategoryID,
		StartAt:     startAt,
		EndAt:       endAt,
	})
	if err != nil {
		c.writeRepositoryError(w, r, err)
		return
	}
	httpapi.OK(w, http.StatusOK, item)
}

func (c *Controller) completeItem(w http.ResponseWriter, r *http.Request) {
	c.changeCompletion(w, r, true)
}

func (c *Controller) uncompleteItem(w http.ResponseWriter, r *http.Request) {
	c.changeCompletion(w, r, false)
}

func (c *Controller) deleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := c.readItemID(w, r)
	if !ok {
		return
	}
	deleted, err := c.repository.DeleteItem(r.Context(), id)
	if err != nil {
		c.writeRepositoryError(w, r, err)
		return
	}
	httpapi.OK(w, http.StatusOK, deleted)
}

func (c *Controller) restoreItem(w http.ResponseWriter, r *http.Request) {
	id, ok := c.readItemID(w, r)
	if !ok {
		return
	}
	item, err := c.repository.RestoreItem(r.Context(), id)
	if err != nil {
		c.writeRepositoryError(w, r, err)
		return
	}
	httpapi.OK(w, http.StatusOK, item)
}

func (c *Controller) readItemID(w http.ResponseWriter, r *http.Request) (string, bool) {
	var request itemIDRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil ||
		strings.TrimSpace(request.ID) == "" {
		writeValidationError(w, r, "缺少 id")
		return "", false
	}
	return strings.TrimSpace(request.ID), true
}

func (c *Controller) changeCompletion(
	w http.ResponseWriter,
	r *http.Request,
	complete bool,
) {
	var request itemIDRequest
	if err := httpapi.ReadJSON(w, r, &request); err != nil ||
		strings.TrimSpace(request.ID) == "" {
		writeValidationError(w, r, "缺少 id")
		return
	}

	var (
		item Item
		err  error
	)
	if complete {
		item, err = c.repository.CompleteItem(r.Context(), strings.TrimSpace(request.ID))
	} else {
		item, err = c.repository.UncompleteItem(r.Context(), strings.TrimSpace(request.ID))
	}
	if err != nil {
		c.writeRepositoryError(w, r, err)
		return
	}
	httpapi.OK(w, http.StatusOK, item)
}

func (c *Controller) writeRepositoryError(
	w http.ResponseWriter,
	r *http.Request,
	err error,
) {
	var validation ValidationError
	switch {
	case errors.Is(err, ErrItemNotFound):
		httpapi.Fail(w, r, http.StatusNotFound, httpapi.ErrorBody{
			Code:      "NOT_FOUND",
			Kind:      "not_found",
			Message:   "事务条目不存在",
			Retryable: false,
		})
	case errors.As(err, &validation):
		writeValidationError(w, r, validation.Message)
	case errors.Is(err, ErrInvalidItemInput):
		writeValidationError(w, r, "参数错误")
	default:
		c.internalError(w, r, "保存事务失败", err)
	}
}

func (c *Controller) internalError(
	w http.ResponseWriter,
	r *http.Request,
	message string,
	err error,
) {
	c.logger.Error(
		"today request failed",
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

func validateTitle(value string) (string, string) {
	title := strings.TrimSpace(value)
	switch {
	case title == "":
		return "", "标题不能为空"
	case utf8.RuneCountInString(title) > maxTitleRunes:
		return "", "标题不能超过 200 个字符"
	default:
		return title, ""
	}
}

func writeValidationError(w http.ResponseWriter, r *http.Request, message string) {
	httpapi.Fail(w, r, http.StatusBadRequest, httpapi.ErrorBody{
		Code:      "VALIDATION_ERROR",
		Kind:      "validation",
		Message:   message,
		Retryable: false,
	})
}
