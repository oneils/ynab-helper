// Package app provides application initialization and configuration.
package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/oneils/ynab-helper/internal/parser"
	"github.com/oneils/ynab-helper/internal/server"
	"github.com/oneils/ynab-helper/internal/sqlite"
	"github.com/oneils/ynab-helper/internal/txn"
	"github.com/oneils/ynab-helper/internal/ynab"
)

// Config holds all application configuration.
type Config struct {
	Addr         string        `long:"addr" env:"ADDR" default:":8080" description:"HTTP service address"`
	WebRoot      string        `long:"web-root" env:"WEB_ROOT" default:"./ui/static" description:"Path to static web assets"`
	YnabAPI      string        `long:"ynab-api" env:"YNAB_API" description:"YNAB API endpoint" default:"https://api.youneedabudget.com/v1"`
	YnabToken    string        `long:"ynab-token" env:"YNAB_TOKEN" description:"YNAB API token"`
	SyncInterval time.Duration `long:"sync-interval" env:"SYNC_INTERVAL" default:"1h" description:"Interval between automatic YNAB syncs"`
	SQLite       sqlite.Config
}

// App represents the application with all its dependencies.
type App struct {
	Config    Config
	DB        *sqlite.DB
	Server    *server.Server
	Scheduler *ynab.Scheduler
}

// New creates a new App with all dependencies wired up.
func New(cfg Config) (*App, error) {
	slog.Info("connecting to SQLite", "path", cfg.SQLite.Path)
	db, err := sqlite.New(cfg.SQLite)
	if err != nil {
		return nil, fmt.Errorf("connecting to SQLite: %w", err)
	}
	slog.Info("connected to SQLite")

	httpClient := &http.Client{Timeout: 30 * time.Second}
	ynabClient := ynab.NewClient(cfg.YnabToken, cfg.YnabAPI, httpClient)

	ynabStore := db.YnabStore()
	txnStore := db.TransactionStore()
	patternStore := db.PatternStore()

	syncer := ynab.NewSyncer(ynabClient, ynabStore, ynabStore, ynabStore, ynabStore, ynabStore)

	timeProvider := parser.RealTimeProvider{}
	parsers := map[string]txn.ReportParser{
		parser.SantanderBankName: parser.NewSantanderParser(santanderConfig(), sha256.New(), timeProvider),
		parser.RevolutBankName:   parser.NewRevolutParser(revolutConfig(), sha256.New(), timeProvider),
		parser.PKOBankName:       parser.NewPKOParser(pkoConfig(), sha256.New(), timeProvider),
		parser.MilleniumBankName: parser.NewMilleniumParser(milleniumConfig(), sha256.New(), timeProvider),
	}

	// Wire smart suggestions
	suggestionEngine := txn.NewSuggestionEngine(patternStore)
	txnProcessor := txn.NewProcessor(parsers, txnStore, ynabStore, ynabClient, suggestionEngine)

	templateCache, err := server.NewTemplateCache()
	if err != nil {
		return nil, fmt.Errorf("creating template cache: %w", err)
	}

	fileStore, err := server.NewTempFileStore()
	if err != nil {
		return nil, fmt.Errorf("creating temp file store: %w", err)
	}

	srv := &server.Server{
		Syncer:        syncer,
		TxnProcessor:  txnProcessor,
		DB:            db,
		FileStore:     fileStore,
		WebRoot:       cfg.WebRoot,
		TemplateCache: templateCache,
	}

	return &App{
		Config:    cfg,
		DB:        db,
		Server:    srv,
		Scheduler: ynab.NewScheduler(syncer, cfg.SyncInterval),
	}, nil
}

// Run starts the application.
func (a *App) Run(ctx context.Context) error {
	go a.Scheduler.Start(ctx)
	return a.Server.Run(ctx, a.Config.Addr)
}

// Close closes all application resources.
func (a *App) Close(ctx context.Context) error {
	return a.DB.Close(ctx)
}

func santanderConfig() parser.Config {
	return parser.Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     2,
		AmountIndex:          5,
		DateFormat:           "02-01-2006",
		BankName:             parser.SantanderBankName,
		ColumnsAmount:        9,
		Header: parser.HeaderCfg{
			HasHeader:      true,
			ValidateHeader: false,
		},
	}
}

func revolutConfig() parser.Config {
	return parser.Config{
		TransactionDateIndex: 2,
		DescriptionIndex:     4,
		AmountIndex:          5,
		FeeIndex:             6,
		CurrencyIndex:        7,
		DateFormat:           "2006-01-02 15:04:05",
		BankName:             parser.RevolutBankName,
		ColumnsAmount:        10,
		Header: parser.HeaderCfg{
			HasHeader: true,
		},
	}
}

func pkoConfig() parser.Config {
	return parser.Config{
		TransactionDateIndex: 0,
		DescriptionIndex:     7,
		AmountIndex:          3,
		DateFormat:           "2006-01-02",
		BankName:             parser.PKOBankName,
		ColumnsAmount:        12,
		Header: parser.HeaderCfg{
			HasHeader:      true,
			ValidateHeader: false,
		},
	}
}

func milleniumConfig() parser.Config {
	return parser.Config{
		TransactionDateIndex: 1,
		DescriptionIndex:     6,
		AmountIndex:          7,
		CurrencyIndex:        10,
		DateFormat:           "2006-01-02",
		BankName:             parser.MilleniumBankName,
		ColumnsAmount:        11,
		Header: parser.HeaderCfg{
			HasHeader: true,
		},
	}
}
