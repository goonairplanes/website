package core

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type AppLogger struct {
	InfoLog  *log.Logger
	ErrorLog *log.Logger
	WarnLog  *log.Logger
}

type GonAirApp struct {
	Router *Router
	Config *Config
	Logger *AppLogger
}

func NewApp() *GonAirApp {
	logger := &AppLogger{
		InfoLog:  log.New(os.Stdout, "✈️ \033[36mINFO\033[0m  ", log.Ldate|log.Ltime),
		ErrorLog: log.New(os.Stderr, "✈️ \033[31mERROR\033[0m ", log.Ldate|log.Ltime),
		WarnLog:  log.New(os.Stdout, "✈️ \033[33mWARN\033[0m  ", log.Ldate|log.Ltime),
	}

	router := NewRouter(logger)

	return &GonAirApp{
		Router: router,
		Config: &AppConfig,
		Logger: logger,
	}
}

func (app *GonAirApp) Init() error {
	startTime := time.Now()

	app.Logger.InfoLog.Printf("Initializing Go on Airplanes...")

	err := app.Router.InitRoutes()
	if err != nil {
		app.Logger.ErrorLog.Printf("Failed to initialize routes: %v", err)
		return fmt.Errorf("failed to initialize routes: %w", err)
	}
	app.Logger.InfoLog.Printf("Routes initialized successfully")

	if AppConfig.InMemoryJS {
		app.Logger.InfoLog.Printf("Initializing JavaScript library cache...")
		if err := FetchAndCacheJSLibraries(); err != nil {
			app.Logger.WarnLog.Printf("Failed to cache JavaScript libraries: %v", err)
			app.Logger.WarnLog.Printf("Falling back to CDN for JavaScript libraries")
		} else {
			app.Logger.InfoLog.Printf("JavaScript libraries cached successfully")
		}
	}

	configureMiddleware := app.getConfigureMiddlewareFunc()
	if configureMiddleware != nil {
		configureMiddleware(app)
		app.Logger.InfoLog.Printf("Middleware configured successfully")
	}

	elapsedTime := time.Since(startTime)
	app.Logger.InfoLog.Printf("Go on Airplanes initialized in %v", elapsedTime.Round(time.Millisecond))

	return nil
}

func (app *GonAirApp) getConfigureMiddlewareFunc() func(*GonAirApp) {
	middlewareConfigPath := filepath.Join(app.Config.AppDir, "middleware.go")
	if _, err := os.Stat(middlewareConfigPath); os.IsNotExist(err) {
		app.Logger.WarnLog.Printf("Middleware configuration file not found at %s", middlewareConfigPath)
		return nil
	}

	return func(app *GonAirApp) {
		app.Router.Use(LoggingMiddleware(app.Logger))
		app.Router.Use(RecoveryMiddleware(app.Logger))

		if app.Config.EnableCORS {
			app.Router.Use(CORSMiddleware(app.Config.AllowedOrigins))
		}

		if app.Config.SSGEnabled {
			app.Router.Use(SSGMiddleware(app.Logger))
			app.Logger.InfoLog.Printf("SSG enabled, static files will be generated in %s", app.Config.SSGDir)
		}
	}
}

func (app *GonAirApp) Start() error {

	port := app.Config.Port

	app.printBanner(port)

	http.DefaultTransport = &http.Transport{
		MaxIdleConns:        1024,
		MaxIdleConnsPerHost: 100,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		DisableCompression:  true,
	}

	mux := http.NewServeMux()

	mux.Handle("/", app.Router)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,

		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,

		MaxHeaderBytes: 1 << 20,
	}

	app.Logger.InfoLog.Printf("Press Ctrl+C to stop the server")

	err := server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		app.Logger.ErrorLog.Printf("Server error: %v", err)
		return fmt.Errorf("server error: %w", err)
	}

	return nil
}

func (app *GonAirApp) printBanner(port string) {
	banner := `
	██████╗  ██████╗   
	██╔════╝ ██╔═══██╗  
	██║  ███╗██║   ██║    
	██║   ██║██║   ██║   
	╚██████╔╝╚██████╔╝    
	╚═════╝  ╚═════╝      
`
	fmt.Print(banner)
	app.Logger.InfoLog.Printf("Go on Airplanes ready for takeoff!")
	app.Logger.InfoLog.Printf("Local:   http://localhost:%s", port)

	interfaces, _ := getNetworkInterfaces()
	if len(interfaces) > 0 {
		for _, ip := range interfaces {
			app.Logger.InfoLog.Printf("Network: http://%s:%s", ip, port)
		}
	}
}

func getNetworkInterfaces() ([]string, error) {
	var ips []string

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {

		if iface.Flags&net.FlagUp == 0 ||
			iface.Flags&net.FlagLoopback != 0 ||
			strings.Contains(iface.Name, "vmnet") ||
			strings.Contains(iface.Name, "vEthernet") ||
			strings.Contains(iface.Name, "vboxnet") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() || ip.To4() == nil {
				continue
			}

			ips = append(ips, ip.String())
		}
	}

	return ips, nil
}
