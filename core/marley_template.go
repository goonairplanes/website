package core

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var renderCache = &sync.Map{}

func (m *Marley) LoadTemplates() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	now := time.Now()
	if !m.cacheExpiry.IsZero() && now.Before(m.cacheExpiry) && AppConfig.TemplateCache {
		return nil
	}

	startTime := time.Now()
	m.Logger.InfoLog.Printf("Loading templates...")

	var wg sync.WaitGroup
	errorCh := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := m.loadComponents(); err != nil {
			errorCh <- err
		}
	}()

	if m.BundleMode {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.bundleAssets(); err != nil {
				errorCh <- err
			}
		}()
	}

	layoutCh := make(chan []byte)
	layoutErrCh := make(chan error, 1)

	go func() {
		layoutContent, err := os.ReadFile(AppConfig.LayoutPath)
		if err != nil {
			layoutErrCh <- fmt.Errorf("failed to load layout template: %w", err)
			return
		}
		layoutCh <- layoutContent
	}()

	wg.Wait()

	select {
	case err := <-errorCh:
		m.Logger.ErrorLog.Printf("Failed to load components: %v", err)
		return err
	default:
	}

	var layoutContent []byte
	select {
	case err := <-layoutErrCh:
		m.Logger.ErrorLog.Printf("Failed to load layout template: %v", err)
		return err
	case layoutContent = <-layoutCh:
		m.Logger.InfoLog.Printf("Layout template loaded successfully")

		m.LayoutMetadata = extractPageMetadata(string(layoutContent), "layout")
		m.Logger.InfoLog.Printf("Layout metadata extracted: %s", m.LayoutMetadata.Title)
	}

	var (
		templatePaths []string
		mu            sync.Mutex
	)

	err := filepath.Walk(AppConfig.AppDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".html" &&
			path != AppConfig.LayoutPath &&
			!strings.HasPrefix(path, AppConfig.ComponentDir) {

			routePath := getRoutePathFromFile(path, AppConfig.AppDir)

			if routePath == "layout" {
				return nil
			}

			if strings.HasPrefix(routePath, "components/") {
				return nil
			}

			mu.Lock()
			templatePaths = append(templatePaths, path)
			mu.Unlock()
		}

		return nil
	})
	if err != nil {
		m.Logger.ErrorLog.Printf("Failed to scan template directories: %v", err)
		return err
	}

	templates := make(map[string]*template.Template)
	pageMetadata := make(map[string]*PageMetadata)

	templateCh := make(chan struct {
		path     string
		tmpl     *template.Template
		metadata *PageMetadata
	}, len(templatePaths))

	semaphore := make(chan struct{}, 4)
	errCh := make(chan error, len(templatePaths))

	var collectorWg sync.WaitGroup
	collectorWg.Add(1)

	go func() {
		defer collectorWg.Done()
		for i := 0; i < len(templatePaths); i++ {
			result := <-templateCh
			templates[result.path] = result.tmpl
			pageMetadata[result.path] = result.metadata

			if AppConfig.SSGEnabled && result.metadata.RenderMode == "ssg" {
				if err := m.generateStaticFile(result.path, result.tmpl, result.metadata); err != nil {
					m.Logger.WarnLog.Printf("Failed to generate static file for %s: %v", result.path, err)
				} else {
					m.Logger.InfoLog.Printf("Generated static file for %s", result.path)
				}
			}
		}
	}()

	for _, path := range templatePaths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			routePath := getRoutePathFromFile(p, AppConfig.AppDir)

			pageContent, err := os.ReadFile(p)
			if err != nil {
				errCh <- fmt.Errorf("failed to read template %s: %w", p, err)
				return
			}

			metadata := extractPageMetadata(string(pageContent), routePath)

			processedContent := processPageContent(string(pageContent), metadata)

			tmpl := template.New("layout")

			_, err = tmpl.Parse(string(layoutContent))
			if err != nil {
				errCh <- fmt.Errorf("failed to parse layout template: %w", err)
				return
			}

			for name, content := range m.ComponentsCache {
				_, err = tmpl.New(name).Parse(content)
				if err != nil {
					errCh <- fmt.Errorf("failed to parse component %s: %w", name, err)
					return
				}
			}

			_, err = tmpl.New("page").Parse(processedContent)
			if err != nil {
				errCh <- fmt.Errorf("failed to parse template %s: %w", p, err)
				return
			}

			templateCh <- struct {
				path     string
				tmpl     *template.Template
				metadata *PageMetadata
			}{routePath, tmpl, metadata}

			m.Logger.InfoLog.Printf("Template loaded: %s → %s (mode: %s)", p, routePath, metadata.RenderMode)
		}(path)
	}

	wg.Wait()
	close(errCh)

	close(templateCh)

	collectorWg.Wait()

	for err := range errCh {
		if err != nil {
			m.Logger.ErrorLog.Printf("Template processing error: %v", err)
			return err
		}
	}

	m.Templates = templates
	m.PageMetadata = pageMetadata

	if AppConfig.TemplateCache {
		m.cacheExpiry = now.Add(m.cacheTTL)
	}

	elapsedTime := time.Since(startTime)
	m.Logger.InfoLog.Printf("Templates loaded successfully in %v", elapsedTime.Round(time.Millisecond))

	return nil
}

func (m *Marley) loadComponents() error {
	componentCache := make(map[string]string)
	componentDir := AppConfig.ComponentDir

	err := filepath.Walk(componentDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && filepath.Ext(path) == ".html" {
			componentContent, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("failed to read component %s: %w", path, err)
			}

			componentName := strings.TrimSuffix(filepath.Base(path), ".html")
			componentCache[componentName] = string(componentContent)
		}

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load components: %w", err)
	}

	m.ComponentsCache = componentCache
	return nil
}

func (m *Marley) RenderTemplate(w http.ResponseWriter, route string, data interface{}) error {
	startTime := time.Now()
	m.mutex.RLock()
	tmpl, ok := m.Templates[route]
	metadata, metaOk := m.PageMetadata[route]
	m.mutex.RUnlock()

	if !ok {
		return fmt.Errorf("template not found: %s", route)
	}

	if !metaOk {
		metadata = &PageMetadata{
			Title:       defaultTitle,
			Description: AppConfig.DefaultMetaTags["description"],
			MetaTags:    make(map[string]string),
			RenderMode:  AppConfig.DefaultRenderMode,
			JSLibrary:   defaultJSLibrary,
		}
	}

	finalMetadata := m.mergeMetadata(route, metadata)

	if cachedContent := m.GetCachedSSGContent(route); cachedContent != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("X-SSG-Cached", "true")
		io.WriteString(w, cachedContent)
		return nil
	}

	cacheKey := "rendered:" + route
	if cachedHTML, found := renderCache.Load(cacheKey); found {
		if renderedHTML, ok := cachedHTML.(string); ok {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("X-Template-Cached", "true")
			io.WriteString(w, renderedHTML)
			return nil
		}
	}

	if AppConfig.LogLevel != "error" {
		m.Logger.InfoLog.Printf("Rendering template %s with mode: %s, title: %s, js: %s",
			route, finalMetadata.RenderMode, finalMetadata.Title, finalMetadata.JSLibrary)
	}

	var buffer strings.Builder
	buffer.Grow(16 * 1024)

	now := time.Now()
	templateData := map[string]interface{}{
		"Metadata":    finalMetadata,
		"Config":      &AppConfig,
		"BuildTime":   now.Format(time.RFC1123),
		"ServerTime":  now.Format(time.RFC1123),
		"CurrentTime": now,
		"Route":       route,
		"Data":        data,
	}

	if m.BundleMode {
		templateData["Bundles"] = m.BundledAssets
	}

	err := tmpl.ExecuteTemplate(&buffer, "layout", templateData)
	if err != nil {
		return fmt.Errorf("error rendering template: %w", err)
	}

	renderedHTML := buffer.String()

	renderedHTML = injectJavaScriptLibraries(renderedHTML, finalMetadata.JSLibrary)

	if AppConfig.TemplateCache && len(renderedHTML) < 64*1024 {
		renderCache.Store(cacheKey, renderedHTML)
	}

	if finalMetadata.RenderMode == "ssg" && AppConfig.SSGEnabled {
		go func() {
			if err := m.generateStaticFile(route, tmpl, metadata); err != nil {
				m.Logger.WarnLog.Printf("Failed to generate static file for %s: %v", route, err)
			}
		}()
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, renderedHTML)

	renderTime := time.Since(startTime)
	if renderTime > 5*time.Millisecond && AppConfig.LogLevel == "debug" {
		m.Logger.InfoLog.Printf("Slow template render: %s took %v", route, renderTime)
	}

	return nil
}

func injectJavaScriptLibraries(html, jsLibrary string) string {
	if jsLibrary == "vanilla" {
		if !AppConfig.DevMode {
			return html
		}
	}

	scriptContent, inMemory, cdnURL := GetJSLibraryContent(jsLibrary)

	var scriptTag string
	if jsLibrary == "alpine" {
		if inMemory {
			scriptTag = fmt.Sprintf("<script defer>%s</script>", scriptContent)
		} else {
			scriptTag = fmt.Sprintf("<script defer src=\"%s\"></script>", cdnURL)
		}
	} else if jsLibrary == "jquery" {
		if inMemory {
			scriptTag = fmt.Sprintf("<script>%s</script>", scriptContent)
		} else {
			scriptTag = fmt.Sprintf("<script src=\"%s\"></script>", cdnURL)
		}
	} else if jsLibrary == "pvue" {
		if inMemory {
			scriptTag = fmt.Sprintf("<script defer>%s</script>", scriptContent)
		} else {
			scriptTag = fmt.Sprintf("<script defer src=\"%s\"></script>", cdnURL)
		}
	}

	if AppConfig.DevMode {
		wsClientJS := GetWebSocketClientJS()
		if scriptTag != "" {
			scriptTag = scriptTag + "\n" + wsClientJS
		} else {
			scriptTag = wsClientJS
		}
	}

	if scriptTag != "" {
		return strings.Replace(html, "</head>", scriptTag+"\n</head>", 1)
	}

	return html
}
