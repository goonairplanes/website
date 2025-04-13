package core

import (
	"html/template"
	"sync"
	"time"
)

type Marley struct {
	Templates       map[string]*template.Template
	Components      map[string]*template.Template
	LayoutTemplate  *template.Template
	ComponentsCache map[string]string
	PageMetadata    map[string]*PageMetadata
	LayoutMetadata  *PageMetadata
	mutex           sync.RWMutex
	cacheExpiry     time.Time
	cacheTTL        time.Duration
	Logger          *AppLogger
	BundledAssets   map[string][]string
	BundleMode      bool

	SSGCache      map[string]SSGCacheEntry
	SSGCacheDir   string
	ssgMutex      *sync.RWMutex
	ssgTaskChan   chan SSGTask
	ssgWorkerPool chan struct{}
}

func NewMarley(logger *AppLogger) *Marley {
	m := &Marley{
		Templates:       make(map[string]*template.Template),
		PageMetadata:    make(map[string]*PageMetadata),
		SSGCache:        make(map[string]SSGCacheEntry),
		SSGCacheDir:     AppConfig.SSGDir,
		ComponentsCache: make(map[string]string),
		BundledAssets:   make(map[string][]string),
		cacheTTL:        15 * time.Minute,
		mutex:           sync.RWMutex{},
		ssgMutex:        &sync.RWMutex{},
		Logger:          logger,
		BundleMode:      false,
	}

	if AppConfig.InMemoryJS {
		logger.InfoLog.Printf("Pre-initializing JavaScript libraries...")
		if err := FetchAndCacheJSLibraries(); err != nil {
			logger.WarnLog.Printf("Failed to pre-cache JavaScript libraries: %v", err)
		}
	}

	return m
}

func (m *Marley) SetCacheTTL(duration time.Duration) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.cacheTTL = duration
	m.cacheExpiry = time.Time{}
	m.Logger.InfoLog.Printf("Template cache TTL set to %v", duration)
}

func (m *Marley) InvalidateCache() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.cacheExpiry = time.Time{}

	m.ssgMutex.Lock()
	m.SSGCache = make(map[string]SSGCacheEntry)
	m.ssgMutex.Unlock()

	renderCache = &sync.Map{}

	m.Templates = make(map[string]*template.Template)
	m.ComponentsCache = make(map[string]string)

	m.Logger.InfoLog.Printf("All caches invalidated: template, render, SSG and component caches cleared")
}
