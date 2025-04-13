package core

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

type PageMetadata struct {
	Title       string
	Description string
	MetaTags    map[string]string
	RenderMode  string
	JSLibrary   string
}

var (
	metaTagRegex    = regexp.MustCompile(`<!--meta:([a-zA-Z0-9_:,\-\s]+)-->`)
	renderModeRegex = regexp.MustCompile(`<!--render:([a-zA-Z]+)-->`)
	titleRegex      = regexp.MustCompile(`<!--title:([^-]+)-->`)
	descRegex       = regexp.MustCompile(`<!--description:([^-]+)-->`)
	jsLibraryRegex  = regexp.MustCompile(`<!--js:\s*([a-zA-Z]+)\s*-->`)

	htmlCommentMetaTagRegex    = regexp.MustCompile(`<!---meta:([a-zA-Z0-9_:,\-\s]+)(?:-->|--->)`)
	htmlCommentRenderModeRegex = regexp.MustCompile(`<!---render:([a-zA-Z]+)(?:-->|--->)`)
	htmlCommentTitleRegex      = regexp.MustCompile(`<!---title:([^-]+)(?:-->|--->)`)
	htmlCommentDescRegex       = regexp.MustCompile(`<!---description:([^-]+)(?:-->|--->)`)
	htmlCommentJSLibraryRegex  = regexp.MustCompile(`<!---js:\s*([a-zA-Z]+)\s*(?:-->|--->)`)

	defaultTitle      = "Go on Airplanes"
	defaultRenderMode = "ssr"
	defaultJSLibrary  = "alpine"
)

type MetadataCache struct {
	cache    map[string]*PageMetadata
	expiry   map[string]time.Time
	ttl      time.Duration
	mutex    sync.RWMutex
	extractC chan extractRequest
}

type extractRequest struct {
	content  string
	filePath string
	result   chan *PageMetadata
}

var metadataCache = NewMetadataCache(8, 30*time.Minute)

func NewMetadataCache(workers int, ttl time.Duration) *MetadataCache {
	mc := &MetadataCache{
		cache:    make(map[string]*PageMetadata),
		expiry:   make(map[string]time.Time),
		ttl:      ttl,
		extractC: make(chan extractRequest, workers*2),
	}

	for i := 0; i < workers; i++ {
		go mc.worker()
	}

	go mc.cleanExpired(30 * time.Minute)

	return mc
}

func (mc *MetadataCache) worker() {
	for req := range mc.extractC {
		metadata := extractPageMetadataInternal(req.content, req.filePath)
		req.result <- metadata
	}
}

func (mc *MetadataCache) cleanExpired(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		mc.mutex.Lock()
		for key, expiry := range mc.expiry {
			if now.After(expiry) {
				delete(mc.cache, key)
				delete(mc.expiry, key)
			}
		}
		mc.mutex.Unlock()
	}
}

func (mc *MetadataCache) Get(key string) (*PageMetadata, bool) {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()

	metadata, ok := mc.cache[key]
	if !ok {
		return nil, false
	}

	if time.Now().After(mc.expiry[key]) {
		return nil, false
	}

	return metadata, true
}

func (mc *MetadataCache) Set(key string, metadata *PageMetadata) {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()

	mc.cache[key] = metadata
	mc.expiry[key] = time.Now().Add(mc.ttl)
}

func extractPageMetadataInternal(content, _ string) *PageMetadata {
	metadata := &PageMetadata{
		Title:       defaultTitle,
		Description: AppConfig.DefaultMetaTags["description"],
		MetaTags:    make(map[string]string, len(AppConfig.DefaultMetaTags)+4),
		RenderMode:  AppConfig.DefaultRenderMode,
		JSLibrary:   defaultJSLibrary,
	}

	for k, v := range AppConfig.DefaultMetaTags {
		metadata.MetaTags[k] = v
	}

	if match := htmlCommentTitleRegex.FindStringSubmatch(content); len(match) > 1 {
		title := strings.TrimSpace(match[1])
		title = strings.TrimSuffix(title, "-")
		title = strings.TrimSpace(title)
		metadata.Title = title
		metadata.MetaTags["og:title"] = title
	} else if match := titleRegex.FindStringSubmatch(content); len(match) > 1 {
		title := strings.TrimSpace(match[1])
		metadata.Title = title
		metadata.MetaTags["og:title"] = title
	}

	if match := htmlCommentDescRegex.FindStringSubmatch(content); len(match) > 1 {
		desc := strings.TrimSpace(match[1])
		desc = strings.TrimSuffix(desc, "-")
		desc = strings.TrimSpace(desc)
		metadata.Description = desc
		metadata.MetaTags["description"] = desc
		metadata.MetaTags["og:description"] = desc
	} else if match := descRegex.FindStringSubmatch(content); len(match) > 1 {
		desc := strings.TrimSpace(match[1])
		metadata.Description = desc
		metadata.MetaTags["description"] = desc
		metadata.MetaTags["og:description"] = desc
	}

	if match := htmlCommentRenderModeRegex.FindStringSubmatch(content); len(match) > 1 {
		mode := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(match[1], "-")))
		if mode == "ssg" {
			metadata.RenderMode = mode
		}
	} else if match := renderModeRegex.FindStringSubmatch(content); len(match) > 1 {
		mode := strings.ToLower(match[1])
		if mode == "ssg" {
			metadata.RenderMode = mode
		}
	}

	for _, match := range htmlCommentMetaTagRegex.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			metaText := strings.TrimSpace(match[1])
			metaText = strings.TrimSuffix(metaText, "-")
			metaText = strings.TrimSpace(metaText)
			if idx := strings.IndexByte(metaText, ':'); idx > 0 {
				key := strings.TrimSpace(metaText[:idx])
				value := strings.TrimSpace(metaText[idx+1:])
				metadata.MetaTags[key] = value
			}
		}
	}

	for _, match := range metaTagRegex.FindAllStringSubmatch(content, -1) {
		if len(match) > 1 {
			metaText := match[1]
			if idx := strings.IndexByte(metaText, ':'); idx > 0 {
				key := strings.TrimSpace(metaText[:idx])
				value := strings.TrimSpace(metaText[idx+1:])
				if _, exists := metadata.MetaTags[key]; !exists {
					metadata.MetaTags[key] = value
				}
			}
		}
	}

	if match := htmlCommentJSLibraryRegex.FindStringSubmatch(content); len(match) > 1 {
		jsLibrary := strings.TrimSpace(match[1])
		jsLibrary = strings.ToLower(jsLibrary)

		switch jsLibrary {
		case "alpine", "jquery", "vanilla", "pvue":
			metadata.JSLibrary = jsLibrary
		}
	} else if match := jsLibraryRegex.FindStringSubmatch(content); len(match) > 1 {
		jsLibrary := strings.ToLower(strings.TrimSpace(match[1]))

		switch jsLibrary {
		case "alpine", "jquery", "vanilla", "pvue":
			metadata.JSLibrary = jsLibrary
		}
	}

	return metadata
}

func extractPageMetadata(content, filePath string) *PageMetadata {
	cacheKey := filePath

	if metadata, found := metadataCache.Get(cacheKey); found {
		return metadata
	}

	if len(content) < 1024 {
		metadata := extractPageMetadataInternal(content, filePath)
		metadataCache.Set(cacheKey, metadata)
		return metadata
	}

	resultChan := make(chan *PageMetadata, 1)
	metadataCache.extractC <- extractRequest{
		content:  content,
		filePath: filePath,
		result:   resultChan,
	}

	metadata := <-resultChan
	metadataCache.Set(cacheKey, metadata)

	return metadata
}

func processPageContent(content string, _ *PageMetadata) string {
	contentBefore := len(content)

	content = titleRegex.ReplaceAllString(content, "")
	content = descRegex.ReplaceAllString(content, "")
	content = metaTagRegex.ReplaceAllString(content, "")
	content = renderModeRegex.ReplaceAllString(content, "")
	content = jsLibraryRegex.ReplaceAllString(content, "")

	content = htmlCommentTitleRegex.ReplaceAllString(content, "")
	content = htmlCommentDescRegex.ReplaceAllString(content, "")
	content = htmlCommentMetaTagRegex.ReplaceAllString(content, "")
	content = htmlCommentRenderModeRegex.ReplaceAllString(content, "")
	content = htmlCommentJSLibraryRegex.ReplaceAllString(content, "")

	content = strings.TrimLeft(content, "\r\n")

	contentAfter := len(content)
	_ = contentBefore - contentAfter

	return content
}

func (m *Marley) mergeMetadata(routePath string, pageMetadata *PageMetadata) *PageMetadata {
	cacheKey := "merge:" + routePath

	if metadata, found := metadataCache.Get(cacheKey); found {
		return metadata
	}

	result := &PageMetadata{
		Title:       defaultTitle,
		Description: AppConfig.DefaultMetaTags["description"],
		MetaTags:    make(map[string]string, len(AppConfig.DefaultMetaTags)+4),
		RenderMode:  AppConfig.DefaultRenderMode,
		JSLibrary:   defaultJSLibrary,
	}

	for k, v := range AppConfig.DefaultMetaTags {
		if k != "description" && k != "og:description" && k != "og:title" {
			result.MetaTags[k] = v
		}
	}

	if m.LayoutMetadata != nil {
		if m.LayoutMetadata.Title != defaultTitle {
			result.Title = m.LayoutMetadata.Title
		}

		if m.LayoutMetadata.Description != AppConfig.DefaultMetaTags["description"] {
			result.Description = m.LayoutMetadata.Description
		}

		for k, v := range m.LayoutMetadata.MetaTags {
			if k != "description" && k != "og:description" && k != "og:title" {
				result.MetaTags[k] = v
			}
		}

		if m.LayoutMetadata.RenderMode != AppConfig.DefaultRenderMode {
			result.RenderMode = m.LayoutMetadata.RenderMode
		}

		if m.LayoutMetadata.JSLibrary != defaultJSLibrary {
			result.JSLibrary = m.LayoutMetadata.JSLibrary
		}
	}

	if pageMetadata.Title != defaultTitle {
		result.Title = pageMetadata.Title
	}

	if pageMetadata.Description != AppConfig.DefaultMetaTags["description"] {
		result.Description = pageMetadata.Description
	}

	for k, v := range pageMetadata.MetaTags {
		if k != "description" && k != "og:description" && k != "og:title" {
			result.MetaTags[k] = v
		}
	}

	if pageMetadata.RenderMode != AppConfig.DefaultRenderMode {
		result.RenderMode = pageMetadata.RenderMode
	}

	if pageMetadata.JSLibrary != defaultJSLibrary {
		result.JSLibrary = pageMetadata.JSLibrary
	}

	result.MetaTags["og:title"] = result.Title

	if result.Description != "" {
		result.MetaTags["description"] = result.Description
		result.MetaTags["og:description"] = result.Description
	}

	if AppConfig.LogLevel == "debug" {
		m.Logger.InfoLog.Printf("Merged metadata for %s: Title='%s', Description='%s...', Mode='%s', JS='%s'",
			routePath,
			result.Title,
			truncateString(result.Description, 30),
			result.RenderMode,
			result.JSLibrary)
	}

	metadataCache.Set(cacheKey, result)

	return result
}
