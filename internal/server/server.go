// Package server provides the HTTP server and routing.
package server

import (
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/oneils/ynab-helper/internal/sqlite"
	"github.com/oneils/ynab-helper/internal/txn"
	"github.com/oneils/ynab-helper/internal/ynab"
)

// Server is the HTTP server.
type Server struct {
	Syncer        *ynab.Syncer
	TxnProcessor  *txn.Processor
	DB            *sqlite.DB
	FileStore     *TempFileStore
	WebRoot       string
	TemplateCache map[string]*template.Template
}

func (s *Server) routes() chi.Router {
	router := chi.NewRouter()

	router.Group(func(r chi.Router) {
		r.Use(middleware.Logger)

		// Health check endpoints
		r.Get("/health", s.healthCheckHandler)
		r.Get("/readiness", s.readinessCheckHandler)

		r.Get("/about", s.aboutViewHandler)
		r.Get("/settings", s.settingsViewHandler)
		r.Get("/import-bank-txns", s.importBankTxnsHandler)
		r.Get("/accounts", s.accountsHandler)

		router.Route("/bank-txns", func(r chi.Router) {
			r.Get("/", s.bankTxnsHandler)
			r.Get("/rows", s.bankTxnRowsHandler)
			r.Get("/{id}/detail", s.detailBankTxnHandler)
			r.Post("/{id}/skip", s.skipBankTxnHandler)

			// Inline editing endpoints
			r.Post("/{id}/save-inline", s.saveInlineTxnHandler)

			// Bulk operations endpoints
			r.Post("/bulk-skip", s.bulkSkipTxnsHandler)

			// API endpoints
			r.Get("/api/payee-suggestions", s.payeeSuggestionsHandler)
			r.Get("/api/category-suggestions", s.categorySuggestionsHandler)
		})

		r.Post("/ynab-budgets-sync", s.syncBudgetsHandler)
		r.Post("/ynab-accs-sync", s.syncAccountsHandler)
		r.Get("/sync-history", s.syncHistoryHandler)
		r.Post("/ynab-categories-sync", s.syncCategoriesHandler)
		r.Post("/ynab-payee-sync", s.syncPayeesHandler)
		r.Post("/ynab-add-txn", s.uploadTxnToYnabHandler)

		r.Post("/upload-bank-txns", s.uploadBankTxnsHandler)
		r.Post("/preview-bank-txns", s.previewBankTxnsHandler)
		r.Post("/confirm-bank-txns", s.confirmBankTxnsHandler)
		r.Post("/cancel-preview", s.cancelPreviewHandler)
		r.Get("/", s.indexHandler)
	})

	s.fileServer(router, "/", truncatedFileSystem{http.Dir(s.WebRoot)})

	return router
}

// Run starts the HTTP server with graceful shutdown support.
func (s *Server) Run(ctx context.Context, addr string) error {
	slog.Info("starting HTTP server", "addr", addr)

	// Start cleanup goroutine for temporary files
	go func() {
		ticker := time.NewTicker(15 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := s.FileStore.CleanupOldFiles(); err != nil {
					slog.Error("failed to cleanup old temporary files", "error", err)
				} else {
					slog.Debug("temporary file cleanup completed")
				}
			case <-ctx.Done():
				slog.Info("stopping temporary file cleanup goroutine")
				return
			}
		}
	}()

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErrors := make(chan error, 1)

	go func() {
		slog.Info("HTTP server listening", "addr", addr)
		serverErrors <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serverErrors:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
		slog.Info("shutdown signal received, starting graceful shutdown")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			slog.Error("graceful shutdown failed", "error", err)
			if closeErr := httpServer.Close(); closeErr != nil {
				slog.Error("failed to force close http server", "error", closeErr)
			}
			return err
		}

		slog.Info("HTTP server gracefully stopped")
	}

	return nil
}

func (s *Server) fileServer(r chi.Router, path string, root http.FileSystem) {
	slog.Info("starting file server", "path", path)
	fs := http.StripPrefix(path, http.FileServer(root))
	path += "*"
	r.Handle(path, http.StripPrefix("/static", fs))
}

// truncatedFileSystem disables directory listings.
type truncatedFileSystem struct {
	fs http.FileSystem
}

func (nfs truncatedFileSystem) Open(path string) (http.File, error) {
	f, err := nfs.fs.Open(path)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}

	if s.IsDir() {
		index := filepath.Join(path, "index.html")
		if _, err := nfs.fs.Open(index); err != nil {
			_ = f.Close()
			return nil, err
		}
	}

	return f, nil
}
