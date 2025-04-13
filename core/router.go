package core

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var paramRegex = regexp.MustCompile(`\[([^/\]]+)\]`)

var apiRegistry = make(map[string]map[string]func(*APIContext))
var apiRegistryMutex sync.RWMutex

func RegisterAPIHandler(path string, method string, handler func(*APIContext)) {
	apiRegistryMutex.Lock()
	defer apiRegistryMutex.Unlock()

	path = normalizePath(path)

	if _, ok := apiRegistry[path]; !ok {
		apiRegistry[path] = make(map[string]func(*APIContext))
	}
	apiRegistry[path][method] = handler

}

type Route struct {
	Path       string
	Handler    http.HandlerFunc
	ParamNames []string
	IsStatic   bool
	IsParam    bool
	Pattern    *regexp.Regexp
	Middleware *MiddlewareChain
}

type Router struct {
	Routes           []Route
	Marley           *Marley
	StaticDir        string
	Logger           *AppLogger
	GlobalMiddleware *MiddlewareChain
	mutex            sync.RWMutex
}

type RouteContext struct {
	Params map[string]string
	Config *Config
}

type APIHandler interface {
	Handler(w http.ResponseWriter, r *http.Request)
}

type APIContext struct {
	Request *http.Request
	Writer  http.ResponseWriter
	Params  map[string]string
	Config  *Config
}

func (ctx *APIContext) Success(data interface{}, statusCode int) {
	RenderSuccess(ctx.Writer, data, statusCode)
}

func (ctx *APIContext) Error(message string, statusCode int) {
	RenderError(ctx.Writer, message, statusCode)
}

func (ctx *APIContext) ParseBody(v interface{}) error {
	return ParseBody(ctx.Request, v)
}

func (ctx *APIContext) QueryParams() map[string]interface{} {
	return ParseJSONParams(ctx.Request)
}

func NewRouter(logger *AppLogger) *Router {
	return &Router{
		Routes:           []Route{},
		Marley:           NewMarley(logger),
		StaticDir:        AppConfig.StaticDir,
		Logger:           logger,
		GlobalMiddleware: NewMiddlewareChain(),
	}
}

func (r *Router) Use(middleware MiddlewareFunc) {
	r.GlobalMiddleware.Use(middleware)
}

func (r *Router) AddRoute(path string, handler http.HandlerFunc, middleware ...MiddlewareFunc) {
	mc := NewMiddlewareChain()
	for _, m := range middleware {
		mc.Use(m)
	}

	paramNames := r.extractParamNames(path)
	isParam := len(paramNames) > 0

	var pattern *regexp.Regexp
	if isParam {
		patternStr := "^" + paramRegex.ReplaceAllString(path, "([^/]+)") + "$"
		pattern = regexp.MustCompile(patternStr)
	}

	r.Routes = append(r.Routes, Route{
		Path:       path,
		Handler:    handler,
		ParamNames: paramNames,
		IsStatic:   false,
		IsParam:    isParam,
		Pattern:    pattern,
		Middleware: mc,
	})
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	startTime := time.Now()
	requestPath := normalizePath(req.URL.Path)

	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if strings.HasPrefix(requestPath, "/static") {
			for _, route := range r.Routes {
				if route.IsStatic {
					route.Handler.ServeHTTP(w, req)
					return
				}
			}
		}

		if strings.HasPrefix(requestPath, "/api") {
			apiRegistryMutex.RLock()
			var matchedHandler func(*APIContext)
			var matchedParams map[string]string

			for registeredPath, methodMap := range apiRegistry {
				params, ok := matchPath(registeredPath, requestPath)
				if ok {
					if handler, methodExists := methodMap[req.Method]; methodExists {
						matchedHandler = handler
						matchedParams = params
					} else if handler, anyMethodExists := methodMap["*"]; anyMethodExists {
						matchedHandler = handler
						matchedParams = params
					}
				}
			}
			apiRegistryMutex.RUnlock()

			if matchedHandler != nil {
				ctx := &APIContext{
					Request: req,
					Writer:  w,
					Params:  matchedParams,
					Config:  &AppConfig,
				}
				matchedHandler(ctx)
				return
			}

			RenderError(w, "API endpoint not found or method not allowed", http.StatusNotFound)
			return
		}

		var pageHandler http.HandlerFunc
		var pageMiddleware *MiddlewareChain

		for _, route := range r.Routes {
			if !route.IsParam && !route.IsStatic {
				if requestPath == route.Path {
					pageHandler = route.Handler
					pageMiddleware = route.Middleware
					break
				}
			}
		}

		if pageHandler == nil {
			for _, route := range r.Routes {
				if route.IsParam && route.Pattern != nil {
					if route.Pattern.MatchString(requestPath) {
						pageHandler = route.Handler
						pageMiddleware = route.Middleware
						break
					}
				}
			}
		}

		if pageHandler != nil {
			if pageMiddleware == nil {
				pageMiddleware = NewMiddlewareChain()
			}
			finalPageRouteHandler := pageMiddleware.Then(pageHandler)
			finalPageRouteHandler.ServeHTTP(w, req)
			return
		}

		r.serveErrorPage(w, req, http.StatusNotFound)
	})

	if r.GlobalMiddleware != nil {
		r.GlobalMiddleware.Then(finalHandler).ServeHTTP(w, req)
	} else {
		finalHandler.ServeHTTP(w, req)
	}

	if AppConfig.LogLevel != "error" {
		go r.logRequest(req, http.StatusOK, time.Since(startTime))
	}
}

func (r *Router) InitRoutes() error {
	startTime := time.Now()
	r.Logger.InfoLog.Printf("Initializing routes...")

	r.Routes = []Route{}

	err := r.Marley.LoadTemplates()
	if err != nil {
		r.Logger.ErrorLog.Printf("Failed to load templates: %v", err)
		return fmt.Errorf("failed to load templates: %w", err)
	}

	r.AddStaticRoute()

	pageRouteCount := 0
	for routePath := range r.Marley.Templates {
		if filepath.Base(routePath) == "layout.html" {
			continue
		}

		paramNames := r.extractParamNames(routePath)
		isParam := len(paramNames) > 0

		var pattern *regexp.Regexp
		if isParam {
			patternStr := "^" + paramRegex.ReplaceAllString(routePath, "([^/]+)") + "$"
			pattern = regexp.MustCompile(patternStr)
		}

		r.Routes = append(r.Routes, Route{
			Path:       routePath,
			Handler:    r.createTemplateHandler(routePath),
			ParamNames: paramNames,
			IsStatic:   false,
			IsParam:    isParam,
			Pattern:    pattern,
			Middleware: NewMiddlewareChain(),
		})

		r.Logger.InfoLog.Printf("Page route registered: %s (params: %v)", routePath, paramNames)
		pageRouteCount++
	}

	apiRouteCount := r.discoverAndLogAPIRoutes()

	elapsedTime := time.Since(startTime)
	r.Logger.InfoLog.Printf("Routes initialized: %d page routes discovered, %d API routes discovered in %v. API handlers registered via init().",
		pageRouteCount, apiRouteCount, elapsedTime.Round(time.Millisecond))

	apiRegistryMutex.RLock()
	r.Logger.InfoLog.Printf("--- Registered API Handlers ---")
	for path, methodMap := range apiRegistry {
		for method := range methodMap {
			r.Logger.InfoLog.Printf("  %s %s", method, path)
		}
	}
	r.Logger.InfoLog.Printf("-----------------------------")
	apiRegistryMutex.RUnlock()

	return nil
}

func (r *Router) discoverAndLogAPIRoutes() int {
	apiBasePath := filepath.Join(AppConfig.AppDir, "api")
	discoveredCount := 0

	if _, err := os.Stat(apiBasePath); os.IsNotExist(err) {
		r.Logger.InfoLog.Printf("No 'api' directory found in '%s'. Skipping API route discovery.", AppConfig.AppDir)
		return 0
	}

	filepath.Walk(apiBasePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			r.Logger.WarnLog.Printf("Error accessing path %q: %v", path, err)
			return err
		}

		if !info.IsDir() && info.Name() == "route.go" {
			relPath, err := filepath.Rel(apiBasePath, filepath.Dir(path))
			if err != nil {
				r.Logger.WarnLog.Printf("Could not get relative path for %s: %v", path, err)
				return nil
			}

			relPath = filepath.ToSlash(relPath)

			apiRoutePath := "/api"
			if relPath != "." {
				parts := strings.Split(relPath, "/")
				for _, part := range parts {
					if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
					}
				}
				apiRoutePath = "/api/" + strings.Join(parts, "/")
			}

			apiRoutePath = normalizePath(apiRoutePath)

			r.Logger.InfoLog.Printf("Discovered potential API route file for: %s", apiRoutePath)
			discoveredCount++
		}
		return nil
	})

	return discoveredCount
}

func (r *Router) AddStaticRoute() {
	staticHandler := http.StripPrefix("/static/", http.FileServer(http.Dir(r.StaticDir)))
	r.Routes = append(r.Routes, Route{
		Path: "/static/",
		Handler: func(w http.ResponseWriter, req *http.Request) {
			if _, err := os.Stat(r.StaticDir); os.IsNotExist(err) {
				r.Logger.ErrorLog.Printf("Static directory '%s' not found", r.StaticDir)
				http.NotFound(w, req)
				return
			}
			staticHandler.ServeHTTP(w, req)
		},
		IsStatic:   true,
		Middleware: NewMiddlewareChain(),
	})
	r.Logger.InfoLog.Printf("Static route registered: /static/ -> %s", r.StaticDir)
}

func (r *Router) createTemplateHandler(routePath string) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		requestPath := normalizePath(req.URL.Path)
		params := extractParamsFromRequest(requestPath, routePath)

		data := map[string]interface{}{
			"Params":     params,
			"Config":     &AppConfig,
			"ServerTime": time.Now().Format(time.RFC1123),
			"BuildTime":  time.Now().Format(time.RFC1123),
			"Route":      routePath,
			"Request": map[string]interface{}{
				"Path":   requestPath,
				"Method": req.Method,
				"Host":   req.Host,
			},
		}

		err := r.Marley.RenderTemplate(w, routePath, data)
		if err != nil {
			r.Logger.ErrorLog.Printf("Template rendering error for request %s (template %s): %v", requestPath, routePath, err)
			if strings.Contains(err.Error(), "template not defined") {
				r.serveErrorPage(w, req, http.StatusNotFound)
			} else {
				r.serveErrorPage(w, req, http.StatusInternalServerError)
			}
			return
		}
	}
}

func (r *Router) serveErrorPage(w http.ResponseWriter, req *http.Request, status int) {
	var errorPage string

	switch status {
	case http.StatusNotFound:
		errorPage = "404"
	case http.StatusInternalServerError:
		errorPage = "500"
	default:
		errorPage = fmt.Sprintf("%d", status)
		if _, exists := r.Marley.Templates["/"+errorPage]; !exists {
			errorPage = "error"
		}
	}

	errorTemplatePath := "/" + errorPage
	if tmpl, exists := r.Marley.Templates[errorTemplatePath]; exists {
		w.WriteHeader(status)
		err := tmpl.ExecuteTemplate(w, "layout", map[string]interface{}{
			"Params": map[string]string{
				"status": fmt.Sprintf("%d", status),
				"path":   req.URL.Path,
				"error":  http.StatusText(status),
			},
			"Config": &AppConfig,
			"Route":  errorTemplatePath,
		})
		if err == nil {
			return
		}
		r.Logger.ErrorLog.Printf("Failed to execute error template %s: %v", errorTemplatePath, err)
	}

	http.Error(w, fmt.Sprintf("%d %s", status, http.StatusText(status)), status)
}

func normalizePath(path string) string {
	if path == "" {
		return "/"
	}
	path = filepath.Clean(path)
	path = filepath.ToSlash(path)

	if path != "/" && strings.HasSuffix(path, "/") {
		path = path[:len(path)-1]
	}
	if path == "" {
		return "/"
	}
	return path
}

func (r *Router) extractParamNames(routePath string) []string {
	matches := paramRegex.FindAllStringSubmatch(routePath, -1)
	var paramNames []string
	for _, match := range matches {
		if len(match) > 1 {
			paramNames = append(paramNames, match[1])
		}
	}
	return paramNames
}

func matchPath(routePath, requestPath string) (map[string]string, bool) {
	routePath = normalizePath(routePath)
	requestPath = normalizePath(requestPath)
	params := make(map[string]string)

	if !strings.Contains(routePath, "[") {
		return params, routePath == requestPath
	}

	patternStr := "^" + paramRegex.ReplaceAllStringFunc(routePath, func(match string) string {
		return "([^/]+)"
	}) + "$"
	pattern := regexp.MustCompile(patternStr)

	matches := pattern.FindStringSubmatch(requestPath)
	if len(matches) == 0 {
		return nil, false
	}

	paramNames := paramRegex.FindAllStringSubmatch(routePath, -1)

	if len(matches)-1 != len(paramNames) {
		return nil, false
	}

	for i, paramMatch := range paramNames {
		if len(paramMatch) > 1 {
			params[paramMatch[1]] = matches[i+1]
		}
	}

	return params, true
}

func extractParamsFromRequest(requestPath, routePath string) map[string]string {
	params, _ := matchPath(routePath, requestPath)
	if params == nil {
		return make(map[string]string)
	}
	return params
}

func (r *Router) logRequest(req *http.Request, status int, duration time.Duration) {
	logLevel := AppConfig.LogLevel
	if logLevel == "debug" || logLevel == "info" {
		statusCode := status
		if statusCode == 0 {
			statusCode = 200
		}
		r.Logger.InfoLog.Printf("Handled: %s %s -> %d (%v)", req.Method, req.URL.Path, statusCode, duration.Round(time.Microsecond))
	} else if status >= 400 && logLevel != "error" {
		r.Logger.WarnLog.Printf("Handled: %s %s -> %d (%v)", req.Method, req.URL.Path, status, duration.Round(time.Microsecond))
	}
}
