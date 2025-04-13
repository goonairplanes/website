package core

import (
	"net/http"
)

type APIHandlerImpl struct{}

func (h *APIHandlerImpl) Handler(ctx *APIContext) {
	switch ctx.Request.Method {
	case http.MethodGet:
		h.handleGet(ctx)
	case http.MethodPost:
		h.handlePost(ctx)
	case http.MethodPut:
		h.handlePut(ctx)
	case http.MethodDelete:
		h.handleDelete(ctx)
	default:
		ctx.Error("Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *APIHandlerImpl) handleGet(ctx *APIContext) {
	ctx.Success(map[string]string{"message": "GET request handled"}, http.StatusOK)
}

func (h *APIHandlerImpl) handlePost(ctx *APIContext) {
	var data map[string]interface{}
	if err := ctx.ParseBody(&data); err != nil {
		ctx.Error("Invalid request body", http.StatusBadRequest)
		return
	}
	ctx.Success(data, http.StatusCreated)
}

func (h *APIHandlerImpl) handlePut(ctx *APIContext) {
	var data map[string]interface{}
	if err := ctx.ParseBody(&data); err != nil {
		ctx.Error("Invalid request body", http.StatusBadRequest)
		return
	}
	ctx.Success(data, http.StatusOK)
}

func (h *APIHandlerImpl) handleDelete(ctx *APIContext) {
	ctx.Success(nil, http.StatusNoContent)
}
