package core

type Config struct {
	AppDir         string
	StaticDir      string
	Port           string
	DevMode        bool
	LiveReload     bool
	DefaultCDNs    bool
	TailwindCDN    string
	JQueryCDN      string
	AlpineJSCDN    string
	PetiteVueCDN   string
	LayoutPath     string
	ComponentDir   string
	AppName        string
	Version        string
	LogLevel       string
	TemplateCache  bool
	EnableCORS     bool
	AllowedOrigins []string
	RateLimit      int
	IsBuiltSystem  bool

	InMemoryJS     bool
	JSLibraryCache map[string]string

	DefaultRenderMode string
	SSGDir            string
	SSGEnabled        bool
	SSGCacheEnabled   bool

	DefaultMetaTags map[string]string
}

var AppConfig = Config{
	AppDir:         "app",
	StaticDir:      "static",
	Port:           "3000",
	DevMode:        true,
	LiveReload:     true,
	DefaultCDNs:    true,
	TailwindCDN:    "https://cdn.tailwindcss.com",
	JQueryCDN:      "https://code.jquery.com/jquery-3.7.1.min.js",
	AlpineJSCDN:    "https://cdn.jsdelivr.net/npm/alpinejs@3.14.9/dist/cdn.min.js",
	PetiteVueCDN:   "https://unpkg.com/petite-vue@0.2.2/dist/petite-vue.iife.js",
	LayoutPath:     "app/layout.html",
	ComponentDir:   "app/components",
	AppName:        "Go on Airplanes",
	Version:        "0.5.3",
	LogLevel:       "info",
	TemplateCache:  true,
	EnableCORS:     false,
	AllowedOrigins: []string{"*"},
	RateLimit:      100,
	IsBuiltSystem:  false,

	InMemoryJS:     true,
	JSLibraryCache: make(map[string]string),

	DefaultRenderMode: "ssr",
	SSGDir:            ".goa/cache",
	SSGEnabled:        true,
	SSGCacheEnabled:   true,

	DefaultMetaTags: map[string]string{
		"viewport":     "width=device-width, initial-scale=1.0",
		"description":  "Go on Airplanes - A modern Go web framework",
		"og:title":     "Go on Airplanes",
		"og:type":      "website",
		"twitter:card": "summary",
	},
}
