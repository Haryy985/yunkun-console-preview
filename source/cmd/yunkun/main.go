// Command yunkun runs the backend: the dual SQLite graph stores,
// the event-driven exploration engine, and the JSON HTTP API consumed by the
// shadcn/ui frontend.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"yunkun/console/agent"
	"yunkun/console/config"
	"yunkun/console/selfupdate"
	"yunkun/console/server"
)

// version is the build version, injected at release time via
// -ldflags "-X main.version=<tag>". Defaults to "dev" for local builds.
var version = "dev"

const banner = "云鲲自动化渗透控制台\n"

// printBanner writes the startup banner + version/runtime info to stdout.
func printBanner(addr string) {
	fmt.Print(banner)
	fmt.Println("  AI 自主渗透测试系统")
	fmt.Printf("  版本 %s  ·  %s/%s  ·  %s  ·  监听 %s\n\n",
		version, runtime.GOOS, runtime.GOARCH, runtime.Version(), addr)
}

// main maps run's result onto the process exit code after cleanup completes.
func main() {
	os.Exit(run())
}

func run() int {
	var (
		addr             = flag.String("addr", ":8787", "HTTP listen address")
		dataDir          = flag.String("data", filepath.Join(config.BaseDir(), "data"), "data directory for SQLite stores (default: data/ next to the executable)")
		proxy            = flag.String("proxy", "127.0.0.1:8788", "traffic recording proxy address (empty to disable)")
		showVersion      = flag.Bool("version", false, "print build version and exit")
		updateJob        = flag.String("offline-files", "", "private offline update job")
		updateOperation  = flag.String("file-operation", "", "private update file operation")
		offlineWorkerJob = flag.String("offline-worker", "", "private Linux/macOS offline update supervisor")
	)
	flag.Parse()
	if *offlineWorkerJob != "" {
		if err := selfupdate.RunOfflineWorker(*offlineWorkerJob); err != nil {
			log.Print(err)
			return 1
		}
		return 0
	}
	if *updateJob != "" {
		if err := selfupdate.OfflineFiles(*updateJob, *updateOperation); err != nil {
			log.Print(err)
			return 1
		}
		return 0
	}
	if *showVersion {
		fmt.Println(version)
		return 0
	}

	// hand the build version to the server package so GET /api/health can report it
	// to the frontend top bar.
	server.BuildVersion = version

	printBanner(*addr)

	// capture backend logs into the in-memory sink (still to stderr) so the /logs
	// page can show a live log stream. Do this first, to catch startup logs too.
	server.StartLogCapture()

	// surface which config file the binary reads (absolute, so `go run`'s relative
	// "config.json" — resolved against the CWD — is unambiguous).
	cfgPath := config.Path()
	if abs, e := filepath.Abs(cfgPath); e == nil {
		cfgPath = abs
	}
	if _, e := os.Stat(cfgPath); e == nil {
		log.Printf("[config] 配置文件: %s", cfgPath)
	} else {
		log.Printf("[config] 配置文件: %s (不存在 — 将仅尝试环境变量 YUNKUN_PG_DSN)", cfgPath)
	}

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, shutdown := shutdownContext(sigCtx)
	defer shutdown(agent.AbortShutdown)

	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Printf("listen: %v", err)
		return 1
	}
	defer listener.Close()

	startup := server.NewStartupHandler()
	httpSrv := &http.Server{Addr: *addr, Handler: startup, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := httpSrv.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("serve: %v", err)
			shutdown(agent.AbortShutdown)
		}
	}()
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(closeCtx)
	}()
	var mgr *server.Manager
	for {
		mgr, err = server.NewManager(*dataDir, *proxy)
		if err == nil {
			break
		}
		log.Printf("open stores failed; retrying in 5s: %v", err)
		select {
		case <-ctx.Done():
			return 1
		case <-time.After(5 * time.Second):
		}
	}
	defer mgr.Close()

	if os.Getenv("YUNKUN_UPDATE_PROBE") == "1" {
		startup.SetReady(server.UpdateProbeHandler(mgr))
		<-ctx.Done()
		return 0
	}

	if count, err := mgr.PG().RecoverRAGIngestionJobs(); err != nil {
		log.Printf("recover RAG ingestion jobs: %v", err)
		return 1
	} else if count > 0 {
		log.Printf("[rag] marked %d interrupted ingestion jobs as failed; documents can be retried", count)
	}

	skillDir := config.SkillDir()
	if abs, err := filepath.Abs(skillDir); err == nil {
		skillDir = abs
	}
	log.Printf("[config] skill 目录: %s", skillDir)
	srv := server.New(ctx, mgr, skillDir, *dataDir, config.BaseDir(), *addr)
	startup.SetReady(srv.Handler())
	log.Printf("Yunkun %s backend ready on %s (data=%s, workers=%d)", version, *addr, *dataDir, mgr.Workers())

	select {
	case <-ctx.Done():
	case <-server.RestartRequested():
		shutdown(agent.AbortShutdown)
	}

	log.Println("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	if selfupdate.OfflineActive(selfupdate.OfflineStatus(os.Getenv("YUNKUN_OFFLINE_ROOT")).Phase) {
		log.Println("update supervisor will restart the console after the verified backup")
	}
	return 0
}

// shutdownContext deliberately does not derive from signalCtx. If it did, the
// parent's plain context.Canceled could win the race before AbortShutdown was
// attached to the child, losing the diagnostic cause in every running Agent.
func shutdownContext(signalCtx context.Context) (context.Context, context.CancelCauseFunc) {
	ctx, shutdown := context.WithCancelCause(context.Background())
	go func() {
		select {
		case <-signalCtx.Done():
			shutdown(agent.AbortShutdown)
		case <-ctx.Done():
		}
	}()
	return ctx, shutdown
}
