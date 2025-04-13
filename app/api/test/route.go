package api

import (
	"goonairplanes/core"
	"net/http"
	"time"
)

func Handler(ctx *core.APIContext) {

	response := map[string]interface{}{
		"message":   "Hello from Go on Airplanes API route!",
		"timestamp": time.Now().Format(time.RFC3339),
		"method":    ctx.Request.Method,
		"path":      ctx.Request.URL.Path,
		"params":    ctx.Params,
		"success":   true,
	}

	ctx.Success(response, http.StatusOK)
}
