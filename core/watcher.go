package core

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/kleeedolinux/socket.go/socket"
)

type FileWatcher struct {
	router        *Router
	watcher       *fsnotify.Watcher
	debounceTimer *time.Timer
	dirs          []string
	logger        *AppLogger
	socketServer  *socket.Server
	enabled       bool
}

func NewFileWatcher(router *Router, logger *AppLogger) (*FileWatcher, error) {
	if AppConfig.IsBuiltSystem {
		logger.InfoLog.Printf("File watcher disabled: running on built system")
		return &FileWatcher{
			router:  router,
			logger:  logger,
			enabled: false,
		}, nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	var socketServer *socket.Server
	if AppConfig.DevMode {

		socketServer = socket.NewServer(
			socket.WithCompression(true),
			socket.WithBufferSize(4096),
			socket.WithPingInterval(25*time.Second),
		)

		socketServer.HandleFunc(socket.EventConnect, func(s socket.Socket, _ interface{}) {

			s.Send("connected", map[string]interface{}{
				"status":     "connected",
				"message":    "Connected to Go on Airplanes LiveReload server",
				"serverTime": time.Now().UnixNano() / int64(time.Millisecond),
			})
		})

		socketServer.HandleFunc("ping", func(s socket.Socket, data interface{}) {
			s.Send("pong", map[string]interface{}{
				"time": time.Now().UnixNano() / int64(time.Millisecond),
			})
		})

		socketServer.HandleFunc(socket.EventDisconnect, func(s socket.Socket, _ interface{}) {

		})
	}

	return &FileWatcher{
		router:       router,
		watcher:      watcher,
		dirs:         []string{AppConfig.AppDir, AppConfig.StaticDir},
		logger:       logger,
		socketServer: socketServer,
		enabled:      true,
	}, nil
}

func (fw *FileWatcher) Start() {
	if !fw.enabled {
		fw.logger.InfoLog.Printf("File watcher is disabled, skipping start")
		return
	}

	fw.logger.InfoLog.Printf("Starting file watcher...")

	for _, dir := range fw.dirs {
		err := fw.watchDir(dir)
		if err != nil {
			fw.logger.ErrorLog.Printf("Error watching directory %s: %v", dir, err)
		} else {
			fw.logger.InfoLog.Printf("Watching directory: %s", dir)
		}
	}

	go fw.watchLoop()
}

func (fw *FileWatcher) Stop() {
	if !fw.enabled {
		return
	}

	if fw.watcher != nil {
		fw.watcher.Close()
		fw.logger.InfoLog.Printf("File watcher stopped")
	}

	if fw.socketServer != nil {
		fw.logger.InfoLog.Printf("Shutting down WebSocket server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := fw.socketServer.Shutdown(ctx); err != nil {
			fw.logger.ErrorLog.Printf("Error shutting down WebSocket server: %v", err)
		}
	}
}

func (fw *FileWatcher) watchLoop() {
	debounceTimeout := 100 * time.Millisecond

	for {
		select {
		case event, ok := <-fw.watcher.Events:
			if !ok {
				return
			}

			if strings.Contains(event.Name, "static\\generated") || strings.Contains(event.Name, "static/generated") {
				continue
			}

			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
				if fw.debounceTimer != nil {
					fw.debounceTimer.Stop()
				}
				fw.debounceTimer = time.AfterFunc(debounceTimeout, func() {
					fw.logger.InfoLog.Printf("File change detected in %s, reloading...", event.Name)

					if fw.router.Marley != nil {
						fw.router.Marley.InvalidateCache()

						if err := fw.cleanGeneratedFiles(); err != nil {
							fw.logger.ErrorLog.Printf("Error cleaning generated files: %v", err)
						} else {
							fw.logger.InfoLog.Printf("Generated HTML files cleaned successfully")
						}

						fw.logger.InfoLog.Printf("Template cache invalidated")
					}

					err := fw.router.InitRoutes()
					if err != nil {
						fw.logger.ErrorLog.Printf("Failed to reload templates: %v", err)
					} else {
						fw.logger.InfoLog.Printf("Templates reloaded successfully")
					}

					if fw.socketServer != nil {
						fw.socketServer.Broadcast("reload", map[string]interface{}{
							"file": event.Name,
							"time": time.Now().UnixNano() / int64(time.Millisecond),
						})
					}
				})
			}
		case err, ok := <-fw.watcher.Errors:
			if !ok {
				return
			}
			fw.logger.ErrorLog.Printf("Watcher error: %v", err)
		}
	}
}

func (fw *FileWatcher) cleanGeneratedFiles() error {
	if !AppConfig.SSGEnabled {
		return nil
	}

	return filepath.Walk(AppConfig.SSGDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".html") {
			if err := os.Remove(path); err != nil {
				fw.logger.WarnLog.Printf("Failed to remove generated file %s: %v", path, err)
				return err
			}
			fw.logger.InfoLog.Printf("Removed generated file: %s", path)
		}

		return nil
	})
}

func (fw *FileWatcher) watchDir(dir string) error {
	if strings.Contains(dir, AppConfig.SSGDir) {
		return nil
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() && (strings.Contains(path, "static\\generated") || strings.Contains(path, "static/generated")) {
			return filepath.SkipDir
		}

		if info.IsDir() {
			return fw.watcher.Add(path)
		}
		return nil
	})
	return err
}

func (fw *FileWatcher) RegisterSocketHandler(mux *http.ServeMux) {
	if !fw.enabled {
		return
	}

	if fw.socketServer != nil && AppConfig.DevMode {
		fw.logger.InfoLog.Printf("Registering WebSocket handler at /socket endpoint for live reload")
		mux.HandleFunc("/socket", fw.socketServer.HandleHTTP)
	} else {
		if !AppConfig.DevMode {
			fw.logger.WarnLog.Printf("WebSocket handler not registered: not in development mode")
		} else if fw.socketServer == nil {
			fw.logger.WarnLog.Printf("WebSocket handler not registered: socket server is nil")
		}
	}
}
