// Package app is the core module's assembly point (the "main control").
// It creates every sub-feature, wires dependencies, mounts routes,
// and starts background jobs. New modules plug in here (3-5 lines each).
package app

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"edgecdn/core/internal/auth"
	"edgecdn/core/internal/authx"
	"edgecdn/core/internal/cert"
	"edgecdn/core/internal/cfg"
	"edgecdn/core/internal/dns"
	"edgecdn/core/internal/dnsprovider"
	"edgecdn/core/internal/logs"
	"edgecdn/core/internal/node"
	"edgecdn/core/internal/oplog"
	"edgecdn/core/internal/outbox"
	"edgecdn/core/internal/parity"
	"edgecdn/core/internal/setting"
	"edgecdn/core/internal/site"
	"edgecdn/core/internal/storex"
	"edgecdn/core/internal/task"
	"edgecdn/core/internal/tenant"
	"edgecdn/core/internal/usage"
	"edgecdn/core/internal/user"
	"edgecdn/core/internal/waf"
	"edgecdn/core/internal/webx"
)

// Run boots the core module and blocks until shutdown.
// staticDir (optional) overrides cfg.StaticDir.
func Run(configPath, staticDir string) error {
	// ---- config & storage ----
	configuration, err := cfg.Load(configPath)
	if err != nil {
		return err
	}
	if staticDir != "" {
		configuration.StaticDir = staticDir
	}
	if err := os.MkdirAll(configuration.DataDir, 0o755); err != nil {
		return err
	}
	db, err := storex.Open(filepath.Join(configuration.DataDir, "edgecdn.db"))
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	defer db.Close()

	// ---- sub-features (order matters only for wiring) ----
	settings := setting.New(db)
	if err := settings.EnsureDefaults(); err != nil {
		return err
	}
	oplogs := oplog.New(db)
	jwtSvc := authx.NewJWT(configuration.JWTSecret)
	admins := user.New(db)
	if err := admins.SeedAdmin("admin", "admin123"); err != nil {
		return err
	}
	tenants := tenant.New(db, settings)
	nodes := node.New(db, oplogs)
	ob := outbox.New(db)
	certs := cert.New(db, settings)
	if err := certs.EnsureCA(); err != nil {
		return fmt.Errorf("ensure CA: %w", err)
	}
	wafSvc := waf.New(db)
	sites := site.New(db, wafSvc, certs, ob, oplogs)
	wafSvc.SetEmitter(sites)
	if err := sites.MigrateLegacy(); err != nil {
		log.Printf("[core] legacy site migration: %v", err)
	}
	usageStore := usage.New(db)
	logsStore := logs.New(db)
	tasksStore := task.New(db)
	dnsStore := dns.New(db)
	dnsProviders := dnsprovider.New(db)

	// ---- HTTP ----
	rt := webx.New()
	rt.SetAuth(func(tok string) (webx.AuthInfo, error) { return jwtSvc.Parse(tok) })
	rt.Public("/api/v1/auth/login", "/api/v1/edge/", "/api/v1/health")
	rt.SetScopeGuard(func(p string) (string, bool) {
		if strings.HasPrefix(p, "/api/v1/admin/") {
			return "admin", true
		}
		if strings.HasPrefix(p, "/api/v1/user/") {
			return "tenant", true
		}
		return "", false
	})

	auth.Register(rt, auth.NewService(jwtSvc, admins, tenants))
	user.RegisterAdmin(rt, admins, oplogs)
	tenant.RegisterAdmin(rt, tenants, oplogs)
	tenant.RegisterUser(rt, tenants)
	node.RegisterAdmin(rt, nodes)
	node.RegisterUser(rt, nodes)
	node.RegisterEdge(rt, nodes, ob, usageStore, settings, db, sites, logsStore)
	site.RegisterAdmin(rt, sites, tenants, usageStore, logsStore)
	site.RegisterUser(rt, sites, tenants, usageStore, logsStore)
	waf.RegisterAdmin(rt, wafSvc, oplogs)
	waf.RegisterUser(rt, wafSvc, oplogs)
	cert.Register(rt, certs, sites, oplogs)
	usage.Register(rt, usageStore)
	setting.Register(rt, settings, oplogs)
	oplog.Register(rt, oplogs)
	task.RegisterAdmin(rt, tasksStore)
	dns.RegisterAdmin(rt, dnsStore, tasksStore)
	dns.RegisterUser(rt, dnsStore, tasksStore)
	dnsprovider.RegisterAdmin(rt, dnsProviders, tasksStore)
	dnsprovider.RegisterUser(rt, dnsProviders, tasksStore)
	parity.RegisterAdmin(rt)

	rt.GET("/api/v1/health", func(c *webx.Context) {
		webx.OK(c.W, map[string]any{"ok": true, "time": time.Now().UTC().Format(time.RFC3339)})
	})
	rt.GET("/api/v1/admin/dashboard", dashboardHandler(db, usageStore, logsStore))
	rt.GET("/api/v1/admin/dashboard/series", func(c *webx.Context) {
		days := webx.QueryInt(c, "days", 14)
		out, err := usageStore.Series(0, 0, days)
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		webx.OK(c.W, out)
	})
	rt.GET("/api/v1/admin/dashboard/top-sites", func(c *webx.Context) {
		days := webx.QueryInt(c, "days", 7)
		out, err := usageStore.SiteTotals(0, days)
		if err != nil {
			webx.Fail(c.W, 500, 500, err.Error())
			return
		}
		// join site names
		type row struct {
			Name   string `json:"name"`
			Domain string `json:"domain"`
			usage.SiteTotal
		}
		rows := []row{}
		for _, t := range out {
			var name, domain string
			_ = db.QueryRow(`SELECT name, domain FROM sites WHERE id = ?`, t.SiteID).Scan(&name, &domain)
			rows = append(rows, row{Name: name, Domain: domain, SiteTotal: t})
		}
		if len(rows) > 10 {
			rows = rows[:10]
		}
		webx.OK(c.W, rows)
	})

	// ---- background jobs ----
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				nodes.MarkOffline(90 * time.Second)
			}
		}
	}()
	go func() {
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ob.GC(7 * 24 * time.Hour)
				tasksStore.GC(7 * 24 * time.Hour)
			}
		}
	}()
	// Task worker: process the queue + periodic cert-expiry check.
	tasksStore.SetHandler(task.TypeCertCheck, func(t *task.Task) error {
		expiring, err := certs.ExpiringSoon(30 * 24 * time.Hour)
		if err != nil {
			return err
		}
		for _, ct := range expiring {
			if _, err := certs.Reissue(ct.ID); err != nil {
				log.Printf("[task] cert reissue %s: %v", ct.Domain, err)
				continue
			}
			log.Printf("[task] re-issued expiring CA cert for %s", ct.Domain)
			_ = sites.EmitByDomain(ct.Domain)
		}
		return nil
	})
	dnsProcessor := dnsprovider.NewProcessor(dnsProviders, dnsStore)
	tasksStore.SetHandler(task.TypeDNSResolve, dnsProcessor.Resolve)
	tasksStore.SetHandler(task.TypeDNSClean, dnsProcessor.Clean)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		certCheck := time.NewTicker(time.Hour)
		defer certCheck.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-certCheck.C:
				if _, err := tasksStore.Enqueue(task.TypeCertCheck, "all", ""); err == nil {
					// deduped if already pending/running
				}
			case <-ticker.C:
			}
			tasksStore.Tick(10)
		}
	}()

	// ---- serve ----
	mux := http.NewServeMux()
	mux.Handle("/api/", rt)
	if configuration.StaticDir != "" && dirExists(configuration.StaticDir) {
		mux.Handle("/", spaHandler(configuration.StaticDir))
		log.Printf("[core] serving web frontend from %s", configuration.StaticDir)
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "EdgeCDN core is running. Frontend: run `pnpm dev` in web/ or start core with -static <web/dist>.")
		})
	}
	srv := &http.Server{Addr: configuration.ListenAddr, Handler: mux}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	log.Printf("[core] listening on %s (data dir: %s)", configuration.ListenAddr, configuration.DataDir)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// dashboardHandler returns the admin overview stats.
func dashboardHandler(db *sql.DB, usageStore *usage.Store, logsStore *logs.Store) webx.Handler {
	return func(c *webx.Context) {
		var tCount, sCount, sOnline, nCount, nOnline int64
		_ = db.QueryRow(`SELECT COUNT(*) FROM tenants`).Scan(&tCount)
		_ = db.QueryRow(`SELECT COUNT(*) FROM sites`).Scan(&sCount)
		_ = db.QueryRow(`SELECT COUNT(*) FROM sites WHERE status = 'normal'`).Scan(&sOnline)
		_ = db.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&nCount)
		_ = db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE status=1`).Scan(&nOnline)
		todayReq, todayBytes, _ := usageStore.TodayTotal()
		var logsToday int64
		day := time.Now().UTC().Format("2006-01-02")
		_ = db.QueryRow(`SELECT COUNT(*) FROM access_logs WHERE ts >= ?`, day).Scan(&logsToday)
		webx.OK(c.W, map[string]any{
			"tenants":          tCount,
			"sites":            sCount,
			"sites_online":     sOnline,
			"nodes":            nCount,
			"nodes_online":     nOnline,
			"today_requests":   todayReq,
			"today_bytes":      todayBytes,
			"logs_today":       logsToday,
		})
	}
}

// spaHandler serves a built SPA: real files as-is, anything else falls back to index.html.
func spaHandler(dir string) http.Handler {
	fs := http.FileServer(http.Dir(dir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := filepath.Join(dir, filepath.Clean(strings.TrimPrefix(r.URL.Path, "/")))
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			fs.ServeHTTP(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, "index.html"))
	})
}

func dirExists(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}
