package app

import (
	"goonairplanes/core"
	"net/http"
)

func ConfigureMiddleware(app *core.GonAirApp) {

	app.Router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			app.Logger.InfoLog.Printf("🛡️ Global Middleware: Processing request for %s", r.URL.Path)
			next.ServeHTTP(w, r)
		})
	})

	app.Router.Use(core.LoggingMiddleware(app.Logger))
	app.Router.Use(core.RecoveryMiddleware(app.Logger))
	app.Router.Use(core.SecureHeadersMiddleware())

	if app.Config.EnableCORS {
		app.Router.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				app.Logger.InfoLog.Printf("🌐 CORS Middleware: Processing request from %s", r.Header.Get("Origin"))
				next.ServeHTTP(w, r)
			})
		})
		app.Router.Use(core.CORSMiddleware(app.Config.AllowedOrigins))
	}

	if app.Config.RateLimit > 0 {
		app.Router.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				app.Logger.InfoLog.Printf("⏱️ Rate Limiting Middleware: Processing request from %s", r.RemoteAddr)
				next.ServeHTTP(w, r)
			})
		})
		app.Router.Use(core.RateLimitMiddleware(app.Config.RateLimit))
	}

	app.Router.AddRoute("/dashboard", nil, func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			app.Logger.InfoLog.Printf("🔒 Auth Middleware: Checking access for %s", r.URL.Path)
			next.ServeHTTP(w, r)
		})
	}, core.AuthMiddleware(func(token string) bool {
		app.Logger.InfoLog.Printf("🔑 Token validation for dashboard access")
		return true
	}))

	
	
	
	

	app.Logger.InfoLog.Printf("✅ Middleware configuration completed")
}
