package core

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type MiddlewareFunc func(http.Handler) http.Handler

type MiddlewareChain struct {
	middlewares []MiddlewareFunc
}

func NewMiddlewareChain() *MiddlewareChain {
	return &MiddlewareChain{
		middlewares: make([]MiddlewareFunc, 0),
	}
}

func (mc *MiddlewareChain) Use(middleware MiddlewareFunc) {
	mc.middlewares = append(mc.middlewares, middleware)
}

func (mc *MiddlewareChain) Then(handler http.Handler) http.Handler {
	for i := len(mc.middlewares) - 1; i >= 0; i-- {
		handler = mc.middlewares[i](handler)
	}
	return handler
}

func LoggingMiddleware(logger *AppLogger) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			logger.InfoLog.Printf("%s %s %s %v", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
		})
	}
}

func RecoveryMiddleware(logger *AppLogger) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.ErrorLog.Printf("Panic recovered: %v", err)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func AuthMiddleware(validateToken func(string) bool) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get("Authorization")
			if token == "" || !validateToken(token) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func CORSMiddleware(allowedOrigins []string) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			for _, allowed := range allowedOrigins {
				if allowed == "*" || allowed == origin {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					break
				}
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func RateLimitMiddleware(requestsPerMinute int) MiddlewareFunc {
	type client struct {
		requests int
		lastTime time.Time
	}

	clients := make(map[string]*client)
	ticker := time.NewTicker(time.Minute)

	go func() {
		for range ticker.C {
			for ip := range clients {
				clients[ip].requests = 0
			}
		}
	}()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := r.RemoteAddr
			c, exists := clients[ip]
			if !exists {
				c = &client{requests: 0, lastTime: time.Now()}
				clients[ip] = c
			}

			if c.requests >= requestsPerMinute {
				http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				return
			}

			c.requests++
			next.ServeHTTP(w, r)
		})
	}
}

func ContextMiddleware(key interface{}, value interface{}) MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), key, value)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func SecureHeadersMiddleware() MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			next.ServeHTTP(w, r)
		})
	}
}


func SSGMiddleware(logger *AppLogger) MiddlewareFunc {
	
	if AppConfig.SSGEnabled {
		
		if !strings.HasPrefix(AppConfig.SSGDir, AppConfig.StaticDir) {
			logger.WarnLog.Printf("SSG directory should be under the static directory. Current configuration: %s, Static: %s",
				AppConfig.SSGDir, AppConfig.StaticDir)

			
			AppConfig.SSGDir = filepath.Join(AppConfig.StaticDir, "generated")
			logger.InfoLog.Printf("Auto-corrected SSG directory to: %s", AppConfig.SSGDir)
		}

		
		if err := os.MkdirAll(AppConfig.SSGDir, 0755); err != nil {
			logger.ErrorLog.Printf("Failed to create SSG directory: %v", err)
		} else {
			logger.InfoLog.Printf("SSG directory created/verified at: %s", AppConfig.SSGDir)

			
			staticGenPath := filepath.Join(AppConfig.StaticDir, "generated")
			if AppConfig.SSGDir != staticGenPath {
				if err := os.MkdirAll(staticGenPath, 0755); err != nil {
					logger.ErrorLog.Printf("Failed to create static generated directory: %v", err)
				}

				
				if AppConfig.SSGDir != staticGenPath {
					logger.InfoLog.Printf("Ensuring SSG directory is accessible via static route")
				}
			}
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}
