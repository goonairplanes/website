package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"goonairplanes/core"
	"log"
	"os"
	"path/filepath"
	"sync"

	_ "goonairplanes/app/api/test"
	_ "goonairplanes/app/api/users"
)

type Configuration struct {
	Server struct {
		Port           string   `json:"port"`
		DevMode        bool     `json:"devMode"`
		IsBuiltSystem  bool     `json:"isBuiltSystem"`
		LiveReload     bool     `json:"liveReload"`
		EnableCORS     bool     `json:"enableCORS"`
		AllowedOrigins []string `json:"allowedOrigins"`
		RateLimit      int      `json:"rateLimit"`
	} `json:"server"`
	Directories struct {
		AppDir       string `json:"appDir"`
		StaticDir    string `json:"staticDir"`
		LayoutPath   string `json:"layoutPath"`
		ComponentDir string `json:"componentDir"`
	} `json:"directories"`
	Performance struct {
		TemplateCache bool `json:"templateCache"`
		InMemoryJS    bool `json:"inMemoryJS"`
	} `json:"performance"`
	SSG struct {
		Enabled      bool   `json:"enabled"`
		CacheEnabled bool   `json:"cacheEnabled"`
		Directory    string `json:"directory"`
	} `json:"ssg"`
	Meta struct {
		AppName         string            `json:"appName"`
		DefaultMetaTags map[string]string `json:"defaultMetaTags"`
	} `json:"meta"`
	CDN struct {
		UseCDN    bool   `json:"useCDN"`
		Tailwind  string `json:"tailwind"`
		JQuery    string `json:"jquery"`
		Alpine    string `json:"alpine"`
		PetiteVue string `json:"petiteVue"`
	} `json:"cdn"`
}

func loadConfig(path string) (*Configuration, error) {
	configFile, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("unable to open config file: %v", err)
	}
	defer configFile.Close()

	var config Configuration
	decoder := json.NewDecoder(configFile)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("unable to parse config file: %v", err)
	}

	return &config, nil
}

func applyConfigToApp(config *Configuration) {
	core.AppConfig.Port = config.Server.Port
	core.AppConfig.DevMode = config.Server.DevMode
	core.AppConfig.IsBuiltSystem = config.Server.IsBuiltSystem
	core.AppConfig.LiveReload = config.Server.LiveReload
	core.AppConfig.EnableCORS = config.Server.EnableCORS
	core.AppConfig.AllowedOrigins = config.Server.AllowedOrigins
	core.AppConfig.RateLimit = config.Server.RateLimit

	core.AppConfig.AppDir = config.Directories.AppDir
	core.AppConfig.StaticDir = config.Directories.StaticDir
	core.AppConfig.LayoutPath = config.Directories.LayoutPath
	core.AppConfig.ComponentDir = config.Directories.ComponentDir

	core.AppConfig.TemplateCache = config.Performance.TemplateCache
	core.AppConfig.InMemoryJS = config.Performance.InMemoryJS

	core.AppConfig.SSGEnabled = config.SSG.Enabled
	core.AppConfig.SSGCacheEnabled = config.SSG.CacheEnabled
	core.AppConfig.SSGDir = config.SSG.Directory

	core.AppConfig.AppName = config.Meta.AppName
	core.AppConfig.DefaultMetaTags = config.Meta.DefaultMetaTags

	core.AppConfig.DefaultCDNs = config.CDN.UseCDN
	core.AppConfig.TailwindCDN = config.CDN.Tailwind
	core.AppConfig.JQueryCDN = config.CDN.JQuery
	core.AppConfig.AlpineJSCDN = config.CDN.Alpine
	core.AppConfig.PetiteVueCDN = config.CDN.PetiteVue
}

func ensureDirectoryExists(path string, name string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		log.Printf("Creating %s directory: %s", name, path)
		return os.MkdirAll(path, 0755)
	}
	return nil
}

func main() {
	configPath := flag.String("config", "config.json", "Path to config file")
	port := flag.String("port", "", "Port to run the server on (overrides config)")
	devMode := flag.Bool("dev", false, "Enable development mode (overrides config)")
	isBuiltSystem := flag.Bool("built", false, "Running on a built/production system (overrides config)")

	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		log.Printf("Warning: Could not load config file: %v", err)
		log.Println("Using default configuration")
	} else {
		log.Printf("Loaded configuration from %s", *configPath)
		applyConfigToApp(config)
	}

	if *port != "" {
		core.AppConfig.Port = *port
		log.Printf("Overriding port with command line value: %s", *port)
	}

	devModeFlag := flag.Lookup("dev")
	if devModeFlag != nil && devModeFlag.Value.String() != devModeFlag.DefValue {
		core.AppConfig.DevMode = *devMode
		log.Printf("Overriding development mode with command line value: %v", *devMode)
	}

	builtFlag := flag.Lookup("built")
	if builtFlag != nil && builtFlag.Value.String() != builtFlag.DefValue {
		core.AppConfig.IsBuiltSystem = *isBuiltSystem
		log.Printf("Overriding built system mode with command line value: %v", *isBuiltSystem)

		if *isBuiltSystem {
			log.Println("Running in production mode on built system")
			core.AppConfig.LiveReload = false
		}
	}

	var wg sync.WaitGroup
	var dirErrors []error

	dirPaths := []struct {
		path string
		name string
	}{
		{core.AppConfig.AppDir, "app"},
		{core.AppConfig.StaticDir, "static"},
		{filepath.Join(core.AppConfig.AppDir, "components"), "components"},
	}

	dirErrorChan := make(chan error, len(dirPaths))

	for _, dir := range dirPaths {
		wg.Add(1)
		go func(path, name string) {
			defer wg.Done()
			if err := ensureDirectoryExists(path, name); err != nil {
				dirErrorChan <- fmt.Errorf("failed to create %s directory: %v", name, err)
			}
		}(dir.path, dir.name)
	}

	wg.Wait()
	close(dirErrorChan)

	for err := range dirErrorChan {
		if err != nil {
			dirErrors = append(dirErrors, err)
		}
	}

	if len(dirErrors) > 0 {
		for _, err := range dirErrors {
			log.Printf("Error: %v", err)
		}
		log.Fatalf("Failed to create one or more required directories")
	}

	app := core.NewApp()

	if err = app.Init(); err != nil {
		log.Fatalf("Failed to initialize Go on Airplanes: %v", err)
	}

	if err = app.Start(); err != nil {
		log.Fatalf("Go on Airplanes server error: %v", err)
	}
}
