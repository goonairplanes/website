package core

import (
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type SSGCacheEntry struct {
	Content    string
	Expiry     time.Time
	Processing bool
}

type SSGTask struct {
	RoutePath  string
	Template   *template.Template
	Metadata   *PageMetadata
	ResultChan chan *SSGResult
}

type SSGResult struct {
	Content string
	Error   error
}

func (m *Marley) initSSGSystem() {
	if m.ssgMutex == nil {
		m.ssgMutex = &sync.RWMutex{}
	}

	if m.ssgWorkerPool == nil && AppConfig.SSGEnabled {
		m.ssgTaskChan = make(chan SSGTask, 100)
		m.ssgWorkerPool = make(chan struct{}, 4)

		for i := 0; i < 4; i++ {
			go m.ssgWorker()
		}
	}
}

func (m *Marley) ssgWorker() {
	for task := range m.ssgTaskChan {
		m.ssgWorkerPool <- struct{}{}
		content, err := m.processSSGTask(task.RoutePath, task.Template, task.Metadata)
		<-m.ssgWorkerPool

		task.ResultChan <- &SSGResult{
			Content: content,
			Error:   err,
		}
	}
}

func (m *Marley) processSSGTask(routePath string, tmpl *template.Template, metadata *PageMetadata) (string, error) {
	finalMetadata := m.mergeMetadata(routePath, metadata)

	relativePath := strings.TrimPrefix(routePath, "/")
	if routePath == "/" {
		relativePath = "index"
	}

	var buffer strings.Builder

	now := time.Now()
	templateData := map[string]interface{}{
		"Metadata":    finalMetadata,
		"Config":      &AppConfig,
		"BuildTime":   now.Format(time.RFC1123),
		"ServerTime":  now.Format(time.RFC1123),
		"CurrentTime": now,
		"Route":       routePath,
	}

	if m.BundleMode {
		templateData["Bundles"] = m.BundledAssets
	}

	err := tmpl.ExecuteTemplate(&buffer, "layout", templateData)
	if err != nil {
		return "", fmt.Errorf("failed to render template to memory: %w", err)
	}

	content := buffer.String()

	
	content = injectJavaScriptLibraries(content, finalMetadata.JSLibrary)

	if AppConfig.SSGCacheEnabled {
		cacheDir := m.SSGCacheDir
		fullPath := filepath.Join(cacheDir, relativePath+".html")

		dirPath := filepath.Dir(fullPath)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			m.Logger.WarnLog.Printf("Failed to create cache directory: %v", err)
		} else {
			err = os.WriteFile(fullPath, []byte(content), 0644)
			if err != nil {
				m.Logger.WarnLog.Printf("Failed to write to cache file: %v", err)
			} else {
				m.Logger.InfoLog.Printf("Cached SSG content to disk: %s", fullPath)
			}
		}
	}

	m.Logger.InfoLog.Printf("Generated SSG content for: %s (mode: %s, size: %d bytes)",
		routePath, finalMetadata.RenderMode, buffer.Len())

	return content, nil
}

func (m *Marley) generateStaticFile(routePath string, tmpl *template.Template, metadata *PageMetadata) error {
	if !AppConfig.SSGEnabled {
		return nil
	}

	m.initSSGSystem()

	m.ssgMutex.RLock()
	entry, exists := m.SSGCache[routePath]
	processing := false
	if exists {
		processing = entry.Processing

		if !entry.Processing && time.Now().Before(entry.Expiry) {
			m.ssgMutex.RUnlock()
			return nil
		}
	}
	m.ssgMutex.RUnlock()

	if processing {
		return nil
	}

	m.ssgMutex.Lock()
	m.SSGCache[routePath] = SSGCacheEntry{
		Processing: true,
		Expiry:     time.Now().Add(30 * time.Minute),
	}
	m.ssgMutex.Unlock()

	resultChan := make(chan *SSGResult, 1)
	task := SSGTask{
		RoutePath:  routePath,
		Template:   tmpl,
		Metadata:   metadata,
		ResultChan: resultChan,
	}

	m.ssgTaskChan <- task

	result := <-resultChan

	if result.Error != nil {
		m.ssgMutex.Lock()
		delete(m.SSGCache, routePath)
		m.ssgMutex.Unlock()
		return result.Error
	}

	m.ssgMutex.Lock()
	m.SSGCache[routePath] = SSGCacheEntry{
		Content:    result.Content,
		Processing: false,
		Expiry:     time.Now().Add(30 * time.Minute),
	}
	m.ssgMutex.Unlock()

	return nil
}

func (m *Marley) GetCachedSSGContent(routePath string) string {
	if m.ssgMutex == nil {
		return ""
	}

	m.ssgMutex.RLock()
	defer m.ssgMutex.RUnlock()

	entry, exists := m.SSGCache[routePath]
	if !exists || entry.Processing || time.Now().After(entry.Expiry) {
		return ""
	}

	return entry.Content
}

func (m *Marley) CleanExpiredSSGCache() {
	if m.ssgMutex == nil {
		return
	}

	m.ssgMutex.Lock()
	defer m.ssgMutex.Unlock()

	now := time.Now()
	for key, entry := range m.SSGCache {
		if now.After(entry.Expiry) {
			delete(m.SSGCache, key)
		}
	}
}

func (m *Marley) bundleAssets() error {
	m.ssgMutex.Lock()
	defer m.ssgMutex.Unlock()

	assetTypes := map[string]string{
		".css": "css",
		".js":  "js",
	}

	for ext, assetType := range assetTypes {
		bundleName := fmt.Sprintf("bundle.%s", assetType)
		bundlePath := filepath.Join(AppConfig.StaticDir, bundleName)

		var bundleContent strings.Builder
		assetFiles := make([]string, 0)

		err := filepath.Walk(AppConfig.StaticDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() && filepath.Ext(path) == ext && !strings.Contains(path, "bundle.") {
				assetFiles = append(assetFiles, path)
			}
			return nil
		})

		if err != nil {
			return fmt.Errorf("failed to scan static directory for %s files: %w", assetType, err)
		}

		if len(assetFiles) == 0 {
			continue
		}

		for _, assetPath := range assetFiles {
			content, err := os.ReadFile(assetPath)
			if err != nil {
				return fmt.Errorf("failed to read asset file %s: %w", assetPath, err)
			}

			relPath, _ := filepath.Rel(AppConfig.StaticDir, assetPath)
			bundleContent.WriteString(fmt.Sprintf("/* %s */\n", relPath))
			bundleContent.Write(content)
			bundleContent.WriteString("\n\n")
		}

		err = os.WriteFile(bundlePath, []byte(bundleContent.String()), 0644)
		if err != nil {
			return fmt.Errorf("failed to write bundle file %s: %w", bundlePath, err)
		}

		m.BundledAssets[assetType] = []string{fmt.Sprintf("/static/%s", bundleName)}
		m.Logger.InfoLog.Printf("Created %s bundle with %d files", assetType, len(assetFiles))
	}

	return nil
}
