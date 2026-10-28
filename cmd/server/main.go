// Command server runs the RAFACTools Planner web application.
package main

import (
	"context"
	"errors"
	"io"
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
	"rafactools-planner/internal/settings"
	"rafactools-planner/internal/web"
)

func main() {
	// Keep a redacted tail of error lines for problem reports.
	log.SetOutput(io.MultiWriter(os.Stderr, web.LogTee()))

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
			if cfg.MasterAdminReset2FA {
				if changed, err := authSvc.ResetTOTPFor(cfg.MasterAdminEmail); err != nil {
					log.Printf("ERROR master admin two-step reset: %v", err)
				} else if changed {
					log.Printf("master admin: two-step sign-in cleared (MASTER_ADMIN_RESET_2FA); remove that setting and set it up again")
				}
			}
		}
	}

	store := programme.NewStore(database)

	// Mail settings come from the environment, overridden by anything the
	// master admin saved on the website. They are resolved afresh at every
	// send, so a saved change needs no restart.
	siteSettings, err := settings.New(database, cfg.SessionKey, config.MailKeys, []string{"SMTP_PASS"})
	if err != nil {
		log.Fatalf("settings: %v", err)
	}
	resolveMail := func() (mail.Sender, error) {
		get, gerr := siteSettings.Getter(os.Getenv)
		if gerr != nil {
			log.Printf("WARNING %v", gerr)
		}
		ms, err := config.ResolveMail(get)
		if err != nil {
			return mail.Sender{}, err
		}
		return mail.Sender{Host: ms.Host, Port: ms.Port, User: ms.User, Pass: ms.Pass, From: ms.From}, nil
	}
	first, err := resolveMail()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := config.RequireMail(config.MailSettings{Host: first.Host}, cfg.AllowLogMail); err != nil {
		log.Fatalf("config: %v", err)
	}
	sender := mail.Dynamic{Resolve: resolveMail, AllowLog: cfg.AllowLogMail}

	// Fail at startup, not on a reader's first click, if the embedded docs
	// are malformed (CI runs the same validation).
	docLib, err := docs.Embedded()
	if err != nil {
		log.Fatalf("docs: %v", err)
	}

	handler := web.NewRouter(&web.Deps{Auth: authSvc, Store: store, Mail: sender, Docs: docLib, Cfg: cfg, Settings: siteSettings, Env: os.Getenv})

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s (SMTP configured: %v)", cfg.Port, first.Host != "")
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
