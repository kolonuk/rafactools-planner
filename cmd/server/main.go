// Command server runs the RAFACTools Planner web application.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rafactools-planner/internal/auth"
	"rafactools-planner/internal/config"
	"rafactools-planner/internal/db"
	"rafactools-planner/internal/docs"
	"rafactools-planner/internal/mail"
	"rafactools-planner/internal/programme"
	"rafactools-planner/internal/web"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer database.Close()

	authSvc := auth.NewService(database, cfg.SessionKey, cfg.CookieSecure, cfg.ClaimDomain, cfg.SkipEmailVerification)
	// Keep the master admin account in step with this deployment's settings.
	// A bad setting is logged loudly but does not stop the site: the account
	// simply is not created (never a weak or wrong one).
	switch {
	case cfg.MasterAdminEmail == "" && cfg.MasterAdminPassword == "":
	case cfg.MasterAdminEmail == "" || cfg.MasterAdminPassword == "":
		log.Printf("ERROR master admin: MASTER_ADMIN_EMAIL and MASTER_ADMIN_PASSWORD must both be set; no master admin configured")
	default:
		if result, err := authSvc.EnsureMasterAdmin(cfg.MasterAdminEmail, cfg.MasterAdminPassword); err != nil {
			log.Printf("ERROR master admin NOT configured: %v", err)
		} else {
			log.Printf("master admin: %s (%s)", result, cfg.MasterAdminEmail)
		}
	}

	store := programme.NewStore(database)
	sender := mail.Sender{Host: cfg.SMTPHost, Port: cfg.SMTPPort, User: cfg.SMTPUser, Pass: cfg.SMTPPass, From: cfg.MailFrom}

	// Fail at startup, not on a reader's first click, if the embedded docs
	// are malformed (CI runs the same validation).
	docLib, err := docs.Embedded()
	if err != nil {
		log.Fatalf("docs: %v", err)
	}

	handler := web.NewRouter(&web.Deps{Auth: authSvc, Store: store, Mail: sender, Docs: docLib, Cfg: cfg})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s (SMTP configured: %v)", cfg.Port, cfg.HasSMTP())
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serve: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
