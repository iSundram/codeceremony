package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/config"
	"github.com/iSundram/codeceremony/backend/internal/httpapi"
	"github.com/iSundram/codeceremony/backend/internal/seed"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

const developmentPassword = "codeceremony-dev"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	passwordHash, err := auth.HashPassword(developmentPassword)
	if err != nil {
		logger.Error("could not hash development password", "error", err)
		os.Exit(1)
	}
	data := store.New(seed.Default(passwordHash))
	tokens := auth.NewSessionManager(time.Duration(cfg.SessionTTLHours)*time.Hour, data)
	api := httpapi.New(cfg, data, tokens, logger)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if cfg.SeedDemoData {
		printDevelopmentIdentities(data, tokens, logger)
	}

	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go api.MailDispatcher().Run(shutdownContext, time.Duration(cfg.MailInterval)*time.Second, cfg.MailBatchSize)
	go api.Webhooks().Run(shutdownContext, time.Duration(cfg.MailInterval)*time.Second, cfg.MailBatchSize)
	logger.Info("mail dispatcher started",
		"sender", api.MailService().Dispatcher().SenderName(),
		"smtp_configured", cfg.SMTPConfigured(),
		"interval_seconds", cfg.MailInterval,
	)
	go func() {
		<-shutdownContext.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			logger.Error("graceful shutdown failed", "error", err)
		}
	}()

	logger.Info("CodeCeremony API listening", "address", cfg.HTTPAddr, "environment", cfg.Environment)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

func printDevelopmentIdentities(data *store.Store, tokens auth.TokenIssuer, logger *slog.Logger) {
	identities := []struct {
		label string
		id    string
	}{
		{label: "organizer", id: "organizer"},
		{label: "judge_a", id: "judge_a"},
		{label: "judge_b", id: "judge_b"},
		{label: "participant", id: "participant"},
		{label: "admin", id: "admin"},
	}
	logger.Info("development seed data ready", "password", developmentPassword)
	for _, identity := range identities {
		user, err := data.UserByID(identity.id)
		if err != nil {
			continue
		}
		token, err := tokens.Issue(user)
		if err != nil {
			continue
		}
		fmt.Printf("%s Cookie: session=%s (%s)\n", identity.label, token, user.Email)
	}
}
