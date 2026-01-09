package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
	"github.com/yjxt/ydms/backend/internal/api"
	"github.com/yjxt/ydms/backend/internal/cache"
	"github.com/yjxt/ydms/backend/internal/config"
	"github.com/yjxt/ydms/backend/internal/database"
	"github.com/yjxt/ydms/backend/internal/ndrclient"
	"github.com/yjxt/ydms/backend/internal/service"
)

const devBinaryPath = "tmp/server-dev"

func main() {
	watch := flag.Bool("watch", false, "enable auto-reload in development mode")
	flag.Parse()

	if *watch {
		if err := runWatchMode(); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatalf("watcher error: %v", err)
		}
		return
	}

	if err := runServer(); err != nil {
		log.Fatalf("server exited with error: %v", err)
	}
}

func runServer() error {
	loadDotEnv()

	cfg := config.Load()
	log.Printf("config loaded: ndr_base=%s default_user=%s db=%s:%d/%s",
		cfg.NDR.BaseURL, cfg.Auth.DefaultUserID, cfg.DB.Host, cfg.DB.Port, cfg.DB.DBName)

	// 连接数据库
	db, err := database.Connect(database.Config{
		Host:     cfg.DB.Host,
		Port:     cfg.DB.Port,
		User:     cfg.DB.User,
		Password: cfg.DB.Password,
		DBName:   cfg.DB.DBName,
		SSLMode:  cfg.DB.SSLMode,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}

	// 运行数据库迁移
	if err := database.AutoMigrateWithDefaults(db, database.AdminDefaults{
		Username:    cfg.Admin.Username,
		Password:    cfg.Admin.Password,
		DisplayName: cfg.Admin.DisplayName,
	}); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// 解析 JWT 过期时间
	jwtExpiry, err := time.ParseDuration(cfg.JWT.Expiry)
	if err != nil {
		log.Printf("warning: invalid JWT expiry duration '%s', using default 24h", cfg.JWT.Expiry)
		jwtExpiry = 24 * time.Hour
	}

	// 创建服务
	cacheProvider := cache.NewNoop()
	ndr := ndrclient.NewClient(ndrclient.NDRConfig{
		BaseURL: cfg.NDR.BaseURL,
		APIKey:  cfg.NDR.APIKey,
		Debug:   cfg.Debug.Traffic,
	})

	// 创建认证相关服务
	userService := service.NewUserService(db)
	svc := service.NewService(cacheProvider, ndr, userService)
	courseService := service.NewCourseService(db, ndr, userService)
	permissionService := service.NewPermissionService(db, userService, ndr)

	// 创建服务层
	apiKeyService := service.NewAPIKeyService(db)

	// 创建 handlers
	headerDefaults := api.HeaderDefaults{
		APIKey:   cfg.NDR.APIKey,
		UserID:   cfg.Auth.DefaultUserID,
		AdminKey: cfg.Auth.AdminKey,
	}
	handler := api.NewHandler(svc, permissionService, headerDefaults)
	authHandler := api.NewAuthHandler(userService, cfg.JWT.Secret, jwtExpiry)
	userHandler := api.NewUserHandler(userService)
	courseHandler := api.NewCourseHandler(courseService)
	apiKeyHandler := api.NewAPIKeyHandler(apiKeyService)
	assetsHandler := api.NewAssetsHandler(svc, headerDefaults)

	// 创建路由器（使用新的配置方式）
	router := api.NewRouterWithConfig(api.RouterConfig{
		Handler:       handler,
		AuthHandler:   authHandler,
		UserHandler:   userHandler,
		CourseHandler: courseHandler,
		APIKeyHandler: apiKeyHandler,
		AssetsHandler: assetsHandler,
		JWTSecret:     cfg.JWT.Secret,
		DB:            db, // 传递 DB 用于 API Key 验证
	})

	server := &http.Server{
		Addr:              cfg.HTTPAddress(),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("backend listening on %s", cfg.HTTPAddress())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	waitForShutdown(server)
	return nil
}

func loadDotEnv() {
	if err := godotenv.Load(".env"); err != nil {
		_ = godotenv.Load()
	}
}

func waitForShutdown(server *http.Server) {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()
	log.Println("shutdown signal received")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
		if err := server.Close(); err != nil {
			log.Printf("forced close failed: %v", err)
		}
	}
	log.Println("server stopped")
}

func runWatchMode() error {
	log.Println("watch: auto-reload enabled, watching for changes...")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := os.MkdirAll(filepath.Dir(devBinaryPath), 0o755); err != nil {
		return fmt.Errorf("prepare tmp directory: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create fsnotify watcher: %w", err)
	}
	defer watcher.Close()

	watchDirs := []string{"cmd", "internal"}
	for _, dir := range watchDirs {
		if err := addWatchRecursive(watcher, dir); err != nil {
			return err
		}
	}

	extraFiles := []string{"go.mod", "go.sum", ".env"}
	for _, file := range extraFiles {
		if _, err := os.Stat(file); err == nil {
			if err := watcher.Add(file); err != nil {
				return fmt.Errorf("watch %s: %w", file, err)
			}
		}
	}

	reloadCh := make(chan struct{}, 1)
	triggerReload := func() {
		select {
		case reloadCh <- struct{}{}:
		default:
		}
	}

	var childMu sync.Mutex
	var child *childProcess

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Create != 0 {
					info, err := os.Stat(event.Name)
					if err == nil && info.IsDir() {
						if err := addWatchRecursive(watcher, event.Name); err != nil {
							log.Printf("watch: failed to add directory %s: %v", event.Name, err)
						}
						continue
					}
				}
				if shouldTriggerReload(event.Name, event.Op) {
					triggerReload()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("watch: error: %v", err)
			case <-ctx.Done():
				return
			}
		}
	}()

	triggerReload()

	for {
		select {
		case <-ctx.Done():
			childMu.Lock()
			stopChild(child)
			childMu.Unlock()
			return context.Canceled
		case <-reloadCh:
			time.Sleep(150 * time.Millisecond)
			childMu.Lock()
			if err := rebuildAndRestart(&child); err != nil {
				log.Printf("watch: %v", err)
			}
			childMu.Unlock()
		}
	}
}

func addWatchRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if name == ".git" || name == "tmp" || name == "vendor" || name == ".gocache" {
			return filepath.SkipDir
		}
		if err := watcher.Add(path); err != nil {
			return fmt.Errorf("watch %s: %w", path, err)
		}
		return nil
	})
}

func shouldTriggerReload(path string, op fsnotify.Op) bool {
	if op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
		return false
	}

	// Ignore editor swap/temp files.
	if strings.HasSuffix(path, "~") || strings.HasSuffix(path, ".swp") || strings.HasSuffix(path, ".tmp") {
		return false
	}

	base := filepath.Base(path)
	switch filepath.Ext(base) {
	case ".go", ".env":
		return true
	}

	return base == "go.mod" || base == "go.sum"
}

func rebuildAndRestart(current **childProcess) error {
	if err := buildBinary(); err != nil {
		return err
	}

	if *current != nil {
		stopChild(*current)
	}

	proc, err := startBinary()
	if err != nil {
		return err
	}

	*current = proc
	return nil
}

func buildBinary() error {
	log.Println("watch: rebuilding backend...")
	cmd := exec.Command("go", "build", "-o", devBinaryPath, "./cmd/server")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build failed: %w", err)
	}
	log.Println("watch: build complete")
	return nil
}

func startBinary() (*childProcess, error) {
	cmd := exec.Command(devBinaryPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start failed: %w", err)
	}

	proc := &childProcess{
		cmd:  cmd,
		done: make(chan error, 1),
	}

	go func() {
		proc.done <- cmd.Wait()
		close(proc.done)
	}()

	log.Println("watch: server started")
	return proc, nil
}

type childProcess struct {
	cmd  *exec.Cmd
	done chan error
}

func stopChild(proc *childProcess) {
	if proc == nil || proc.cmd == nil || proc.cmd.Process == nil {
		return
	}

	// Try a graceful stop first.
	if runtime.GOOS == "windows" {
		_ = proc.cmd.Process.Kill()
	} else {
		_ = proc.cmd.Process.Signal(syscall.SIGINT)
	}

	select {
	case <-proc.done:
	case <-time.After(3 * time.Second):
		_ = proc.cmd.Process.Kill()
		<-proc.done
	}

	log.Println("watch: server stopped")
}
