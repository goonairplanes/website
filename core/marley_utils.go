package core

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var jsLibrariesLoaded sync.Once
var jsLibraryMutex sync.RWMutex

func GetWebSocketClientJS() string {
	if !AppConfig.DevMode {
		return ""
	}

	return `
<script>
(function() {
  console.log("[LiveReload] Initializing WebSocket live reload...");
  
  let socket;
  let reconnectTimer;
  let reconnectAttempts = 0;
  const maxReconnectAttempts = 10;
  const reconnectDelay = 1000; // Start with 1 second delay
  const maxReconnectDelay = 30000; // Max 30 second delay

  function connect() {
    try {
      // Clear any existing socket
      if (socket) {
        socket.close();
      }
      
      const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
      const wsUrl = protocol + '//' + location.host + '/socket';
      
      console.log("[LiveReload] Connecting to WebSocket server at:", wsUrl);
      
      // Create new WebSocket with explicit timeout handling
      socket = new WebSocket(wsUrl);
      
      // Set connection timeout
      const connectionTimeout = setTimeout(() => {
        if (socket.readyState !== WebSocket.OPEN) {
          console.error("[LiveReload] Connection timeout, will retry");
          socket.close();
        }
      }, 5000);
      
      socket.addEventListener('open', function(event) {
        clearTimeout(connectionTimeout);
        console.log("[LiveReload] WebSocket connection established successfully");
        reconnectAttempts = 0;
        
        // Send an initial ping to verify connection
        try {
          socket.send(JSON.stringify({type: "ping"}));
        } catch (e) {
          console.error("[LiveReload] Failed to send initial ping:", e);
        }
      });
      
      socket.addEventListener('message', function(event) {
        console.log("[LiveReload] Message received:", event.data);
        
        try {
          const messageData = JSON.parse(event.data);
          
          // Handle reload command from server
          if (messageData.event === 'reload') {
            console.log("[LiveReload] Reloading page due to file change:", messageData.data.file);
            window.location.reload();
          }
        } catch (e) {
          console.error("[LiveReload] Error parsing message:", e);
          console.error("[LiveReload] Raw message:", event.data);
        }
      });
      
      socket.addEventListener('close', function(event) {
        clearTimeout(connectionTimeout);
        console.log("[LiveReload] Connection closed. Code:", event.code, "Reason:", event.reason);
        
        if (reconnectAttempts < maxReconnectAttempts) {
          const delay = Math.min(reconnectDelay * Math.pow(1.5, reconnectAttempts), maxReconnectDelay);
          console.log("[LiveReload] Reconnecting in", delay, "ms...");
          
          clearTimeout(reconnectTimer);
          reconnectTimer = setTimeout(function() {
            reconnectAttempts++;
            connect();
          }, delay);
        } else {
          console.error("[LiveReload] Reconnection failed after", maxReconnectAttempts, "attempts - LiveReload disabled");
        }
      });
      
      socket.addEventListener('error', function(event) {
        console.error("[LiveReload] WebSocket error:", event);
      });
    } catch (e) {
      console.error("[LiveReload] Error setting up WebSocket:", e);
      
      // Try to reconnect after a delay
      if (reconnectAttempts < maxReconnectAttempts) {
        const delay = Math.min(reconnectDelay * Math.pow(1.5, reconnectAttempts), maxReconnectDelay);
        clearTimeout(reconnectTimer);
        reconnectTimer = setTimeout(function() {
          reconnectAttempts++;
          connect();
        }, delay);
      }
    }
  }
  
  // Initialize connection when document is ready
  if (document.readyState === 'complete' || document.readyState === 'interactive') {
    setTimeout(connect, 500); // Slight delay to ensure DOM is fully ready
  } else {
    document.addEventListener('DOMContentLoaded', function() {
      setTimeout(connect, 500);
    });
  }
})();
</script>
`
}

func FetchAndCacheJSLibraries() error {
	if !AppConfig.InMemoryJS {
		return nil
	}

	var loadErr error
	jsLibrariesLoaded.Do(func() {
		var wg sync.WaitGroup

		libraries := map[string]string{
			"jquery": AppConfig.JQueryCDN,
			"alpine": AppConfig.AlpineJSCDN,
			"pvue":   AppConfig.PetiteVueCDN,
		}

		errCh := make(chan error, len(libraries))

		for lib, url := range libraries {
			wg.Add(1)
			go func(libName, libURL string) {
				defer wg.Done()

				client := &http.Client{
					Timeout: 10 * time.Second,
				}
				resp, err := client.Get(libURL)
				if err != nil {
					errCh <- fmt.Errorf("failed to fetch %s from %s: %w", libName, libURL, err)
					return
				}
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					errCh <- fmt.Errorf("failed to fetch %s from %s: status code %d", libName, libURL, resp.StatusCode)
					return
				}

				content, err := io.ReadAll(resp.Body)
				if err != nil {
					errCh <- fmt.Errorf("failed to read %s content: %w", libName, err)
					return
				}

				jsLibraryMutex.Lock()
				AppConfig.JSLibraryCache[libName] = string(content)
				jsLibraryMutex.Unlock()
			}(lib, url)
		}

		wg.Wait()
		close(errCh)

		for err := range errCh {
			if err != nil {
				loadErr = err
				break
			}
		}
	})

	return loadErr
}

func GetJSLibraryContent(library string) (string, bool, string) {
	if !AppConfig.InMemoryJS {
		switch library {
		case "alpine":
			return "", false, AppConfig.AlpineJSCDN
		case "jquery":
			return "", false, AppConfig.JQueryCDN
		case "pvue":
			return "", false, AppConfig.PetiteVueCDN
		default:
			return "", false, ""
		}
	}

	jsLibraryMutex.RLock()
	defer jsLibraryMutex.RUnlock()

	content, exists := AppConfig.JSLibraryCache[library]
	if exists {
		return content, true, ""
	}

	switch library {
	case "alpine":
		return "", false, AppConfig.AlpineJSCDN
	case "jquery":
		return "", false, AppConfig.JQueryCDN
	case "pvue":
		return "", false, AppConfig.PetiteVueCDN
	default:
		return "", false, ""
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func getRoutePathFromFile(fullPath, basePath string) string {
	fullPath = filepath.ToSlash(fullPath)
	basePath = filepath.ToSlash(basePath)

	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}

	relativePath := fullPath
	if strings.HasPrefix(fullPath, basePath) {
		relativePath = strings.TrimPrefix(fullPath, basePath)
	}

	relativePath = strings.TrimSuffix(relativePath, ".html")

	if relativePath == "index" {
		return "/"
	} else if strings.HasSuffix(relativePath, "/index") {
		relativePath = strings.TrimSuffix(relativePath, "/index")
		if relativePath == "" {
			return "/"
		}
	}

	if relativePath != "/" && !strings.HasPrefix(relativePath, "/") {
		relativePath = "/" + relativePath
	}

	return relativePath
}
