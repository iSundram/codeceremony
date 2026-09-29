package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/iSundram/codeceremony/backend/internal/auth"
	"github.com/iSundram/codeceremony/backend/internal/config"
	"github.com/iSundram/codeceremony/backend/internal/domain"
	"github.com/iSundram/codeceremony/backend/internal/fixtures"
	"github.com/iSundram/codeceremony/backend/internal/httpapi"
	"github.com/iSundram/codeceremony/backend/internal/persistence"
	"github.com/iSundram/codeceremony/backend/internal/seed"
	"github.com/iSundram/codeceremony/backend/internal/store"
)

func main() {
	// The runtime image is distroless: there is no shell, so `docker run
	// ... sh -c curl ...` cannot work. These two flags give the container
	// something it can execute, which is how the compose healthcheck probes the
	// service without adding curl to a deliberately empty image.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-healthcheck", "--healthcheck":
			os.Exit(healthcheck())
		case "-version", "--version":
			fmt.Printf("codeceremony %s (snapshot v%d)\n", Version, store.SnapshotVersion)
			os.Exit(0)
		}
	}
	run()
}

// healthcheck probes the service's own liveness route on the address it is
// configured to serve, so it cannot drift from a hardcoded port.
func healthcheck() int {
	address := os.Getenv("HTTP_ADDR")
	if address == "" {
		address = ":8080"
	}
	if strings.HasPrefix(address, ":") {
		address = "127.0.0.1" + address
	}
	client := &http.Client{Timeout: 4 * time.Second}
	response, err := client.Get("http://" + address + "/healthz")
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		return 1
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: /healthz returned %d\n", response.StatusCode)
		return 1
	}
	return 0
}

func run() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	passwordHash, err := auth.HashPassword(cfg.SeedPassword)
	if err != nil {
		logger.Error("could not hash seed password", "error", err)
		os.Exit(1)
	}
	portal := store.New(seed.Data{})
	tokens := auth.NewSessionManager(time.Duration(cfg.SessionTTLHours)*time.Hour, portal)

	// Durability first. A data directory that already holds state wins over the
	// seed, because an organizer's submissions are not a demo artefact. The
	// seed is only used for a cold boot, and is applied after the restore so
	// that accounts needing a password always end up with one.
	journal := persistence.New(portal, dataFile(cfg.DataDir), cfg.PersistInterval, logger)
	if err := ensureDataDir(cfg.DataDir); err != nil {
		logger.Error("the data directory is not usable", "dir", cfg.DataDir, "error", err,
			"hint", "point DATA_DIR at a directory this process can write to")
		os.Exit(1)
	}
	restored, err := journal.Restore()
	if err != nil {
		logger.Error("could not read the data directory, starting from the seed instead",
			"path", journal.Path(), "error", err)
	}
	if !restored {
		portal.SeedFrom(bootData(cfg, passwordHash, logger))
	} else {
		logger.Info("running on restored data, not seeding")
	}

	var identities []seed.Resolved
	if cfg.SeedDemoData {
		identities, err = issueSeededIdentities(portal, tokens)
		if err != nil {
			logger.Error("could not issue seeded identities", "error", err)
			os.Exit(1)
		}
	}
	api := httpapi.New(cfg, portal, tokens, logger)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if cfg.SeedDemoData {
		printSeededIdentities(identities, cfg)
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

	go journal.Run(shutdownContext.Done())
	logger.Info("CodeCeremony API listening",
		"address", cfg.HTTPAddr, "environment", cfg.Environment,
		"data_file", journal.Path(), "restored", restored, "seeded", cfg.SeedDemoData)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("server stopped unexpectedly", "error", err)
		os.Exit(1)
	}
}

// bootData builds the seed set. When a shared fixture file is present it is
// loaded, because the point of the shared file is that every entrant's portal
// shows the same invented hackathon; the portal's own demo event is then added
// alongside it. With no fixture file the portal still boots on its built-in
// seed, so a clone with the file absent is runnable rather than broken.
func bootData(cfg config.Config, passwordHash string, logger *slog.Logger) seed.Data {
	if cfg.FixturesPath == "" {
		logger.Warn("no fixtures file configured, seeding the built-in demo data only",
			"hint", "set FIXTURES_PATH to the shared fixtures.json")
		return seed.WithDemoEvent(seed.Default(passwordHash), time.Now())
	}
	data, err := fixtures.Load(cfg.FixturesPath, passwordHash)
	if err != nil {
		// A corrupt or missing fixture file must not stop the portal coming up:
		// an organizer needs a working portal to debug, and the error is on
		// stderr where it will be seen.
		logger.Error("could not load fixtures, falling back to the built-in seed",
			"path", cfg.FixturesPath, "error", err)
		return seed.WithDemoEvent(seed.Default(passwordHash), time.Now())
	}
	logger.Info("loaded shared fixtures",
		"path", cfg.FixturesPath,
		"tracks", len(data.Tracks), "teams", len(data.Teams),
		"submissions", len(data.Submissions), "reviews", len(data.Reviews),
		"duplicates_flagged", len(data.Duplicates))
	return seed.WithDemoEvent(data, time.Now())
}

// Version is the build identity reported by -version.
const Version = "0.1.0-dogfood"

// seededTokenIssuer is the slice of the session manager the wiring needs: it
// must be able to mint a session for a token it already knows rather than one
// it generates. Declaring it here rather than widening auth.TokenIssuer keeps
// the stateless test issuer from growing a method it has no use for.
type seededTokenIssuer interface {
	IssueWithToken(user domain.User, token string) error
}

// issueSeededIdentities mints a session for every seeded identity using its
// fixed token, so the headers in .dogfood.toml work against a cold container
// with no login step. It lives in the composition root because the seed table
// is data, and wiring data to a live store is not.
//
// Each label resolves against whichever panel was actually seeded, so the same
// credential table serves a fixture-seeded portal and a demo-seeded one.
func issueSeededIdentities(portal *store.Store, tokens seededTokenIssuer) ([]seed.Resolved, error) {
	identities := seed.SeededIdentities()
	resolved := make([]seed.Resolved, 0, len(identities))
	for _, identity := range identities {
		user, userID, err := resolveIdentity(portal, identity)
		if err != nil {
			return nil, err
		}
		// A role change in the seed would silently hand out a token for a role
		// the credential table does not document, which is exactly the drift
		// that makes a documented table untrustworthy.
		if user.Role != identity.Role {
			return nil, fmt.Errorf("identity %s resolves to %s with role %s but the credential table claims %s",
				identity.Label, userID, user.Role, identity.Role)
		}
		if err := tokens.IssueWithToken(user, identity.Token); err != nil {
			return nil, fmt.Errorf("could not issue identity %s: %w", identity.Label, err)
		}
		bound := identity
		bound.Email = user.Email
		resolved = append(resolved, seed.Resolved{Identity: bound, UserID: userID})
	}
	return resolved, nil
}

func resolveIdentity(portal *store.Store, identity seed.Identity) (domain.User, string, error) {
	var tried []string
	for _, candidate := range identity.Candidates {
		tried = append(tried, candidate)
		user, err := portal.UserByID(candidate)
		if err == nil {
			return user, candidate, nil
		}
	}
	return domain.User{}, "", fmt.Errorf("identity %s has no seeded account; looked for %s",
		identity.Label, strings.Join(tried, ", "))
}

// printSeededIdentities writes the fixed auth headers the acceptance checker
// attaches. They are stable across boots, which is the whole point: the
// checker never logs in, so the same header in .dogfood.toml has to keep
// working on a cold container.
func printSeededIdentities(identities []seed.Resolved, cfg config.Config) {
	fmt.Println()
	fmt.Println("  CodeCeremony seeded identities (SEED_DEMO_DATA is on)")
	fmt.Println("  These tokens are fixed so tools can attach them without logging in.")
	fmt.Println("  They are credentials for a demo portal seeded with invented data.")
	fmt.Println()
	for _, identity := range identities {
		fmt.Printf("    %-18s %-11s %-28s %s\n", identity.Label, identity.Role, identity.UserID, identity.Email)
	}
	fmt.Println()
	fmt.Println("  [auth] table for .dogfood.toml:")
	for _, line := range seed.TokenTable(identities) {
		fmt.Println("    " + line)
	}
	fmt.Println()
	fmt.Printf("    login password (all seeded accounts): %s\n", cfg.SeedPassword)
	fmt.Println()
}

// ensureDataDir creates the data directory and proves it is writable before
// any other work happens. A container whose volume is mounted with the wrong
// ownership otherwise fails much later, on the first write, with an error that
// says nothing about the cause.
func ensureDataDir(dir string) error {
	if dir == "" {
		return fmt.Errorf("DATA_DIR must not be empty")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	probe, err := os.CreateTemp(dir, ".writable-*")
	if err != nil {
		return err
	}
	name := probe.Name()
	probe.Close()
	return os.Remove(name)
}

// dataFile is the single file that holds all durable portal state.
func dataFile(dataDir string) string {
	return filepath.Join(dataDir, "portal.json")
}
