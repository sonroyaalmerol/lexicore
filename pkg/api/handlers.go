package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"codeberg.org/lexicore/lexicore/pkg/controller"
	"codeberg.org/lexicore/lexicore/pkg/manifest"
	"codeberg.org/lexicore/lexicore/pkg/store"
	"go.uber.org/zap"
)

func SetupRoutes(mux *http.ServeMux, ctx context.Context, db *store.EtcdStore, mgr *controller.Manager, logger *zap.Logger) {
	p := manifest.NewParser()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/apis/lexicore.io/v1/identitysources", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(mgr.GetIdentitySources()); err != nil {
				logger.Error("Failed to encode response", zap.Error(err))
				http.Error(w, "Internal error", http.StatusInternalServerError)
			}

		case http.MethodPost:
			b, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}

			m, err := p.Parse(b)
			if err != nil {
				logger.Error("Failed to parse manifest", zap.Error(err))
				http.Error(w, "Invalid manifest", http.StatusBadRequest)
				return
			}

			src, ok := m.(*manifest.IdentitySource)
			if !ok {
				http.Error(w, "Invalid manifest type", http.StatusBadRequest)
				return
			}

			if err := db.Put(ctx, "identitysources", src.Name, src); err != nil {
				logger.Error("Store put failed", zap.Error(err))
				http.Error(w, "Store error", http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "created", "name": src.Name})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/apis/lexicore.io/v1/identitysources/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/apis/lexicore.io/v1/identitysources/")
		parts := strings.Split(path, "/")

		if len(parts) < 1 || parts[0] == "" {
			http.Error(w, "Identity source name required", http.StatusBadRequest)
			return
		}

		sourceName := parts[0]

		if len(parts) == 2 && parts[1] == "details" {
			if r.Method != http.MethodGet {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			src, ok := mgr.GetIdentitySource(sourceName)
			if !ok {
				http.Error(w, fmt.Sprintf("Identity source %q not found", sourceName), http.StatusNotFound)
				return
			}

			logger.Info("Fetching details for identity source",
				zap.String("source", sourceName),
				zap.String("remote_addr", r.RemoteAddr))

			detailCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			identities, err := src.GetIdentities(detailCtx)
			if err != nil {
				logger.Error("Failed to fetch identities",
					zap.String("source", sourceName),
					zap.Error(err))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{
					"error": fmt.Sprintf("Failed to fetch identities: %v", err),
				})
				return
			}

			groups, err := src.GetGroups(detailCtx)
			if err != nil {
				logger.Error("Failed to fetch groups",
					zap.String("source", sourceName),
					zap.Error(err))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(map[string]string{
					"error": fmt.Sprintf("Failed to fetch groups: %v", err),
				})
				return
			}

			response := map[string]any{
				"source": sourceName,
				"identities": map[string]any{
					"count": len(identities),
					"items": identities,
				},
				"groups": map[string]any{
					"count": len(groups),
					"items": groups,
				},
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			if err := json.NewEncoder(w).Encode(response); err != nil {
				logger.Error("Failed to encode response", zap.Error(err))
			}
			return
		}

		http.Error(w, "Not found", http.StatusNotFound)
	})

	mux.HandleFunc("/apis/lexicore.io/v1/synctargets", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			if err := json.NewEncoder(w).Encode(mgr.GetSyncTargets()); err != nil {
				logger.Error("Failed to encode response", zap.Error(err))
				http.Error(w, "Internal error", http.StatusInternalServerError)
			}

		case http.MethodPost:
			b, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Failed to read body", http.StatusBadRequest)
				return
			}

			m, err := p.Parse(b)
			if err != nil {
				logger.Error("Failed to parse manifest", zap.Error(err))
				http.Error(w, "Invalid manifest", http.StatusBadRequest)
				return
			}

			target, ok := m.(*manifest.SyncTarget)
			if !ok {
				http.Error(w, "Invalid manifest type", http.StatusBadRequest)
				return
			}

			if err := db.Put(ctx, "synctargets", target.Name, target); err != nil {
				logger.Error("Store put failed", zap.Error(err))
				http.Error(w, "Store error", http.StatusInternalServerError)
				return
			}

			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "created", "name": target.Name})

		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/apis/lexicore.io/v1/synctargets/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/apis/lexicore.io/v1/synctargets/")
		parts := strings.Split(path, "/")

		if len(parts) != 2 || parts[1] != "reconcile" {
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		targetName := parts[0]
		if targetName == "" {
			http.Error(w, "Target name required", http.StatusBadRequest)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("Manual reconciliation triggered",
			zap.String("target", targetName),
			zap.String("remote_addr", r.RemoteAddr))

		if err := mgr.TriggerReconciliation(targetName); err != nil {
			logger.Error("Failed to trigger reconciliation",
				zap.String("target", targetName),
				zap.Error(err))

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "error",
				"error":  err.Error(),
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "queued",
			"target": targetName,
		})
	})

	mux.HandleFunc("/apis/lexicore.io/v1/reconcile", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		logger.Info("Bulk reconciliation triggered", zap.String("remote_addr", r.RemoteAddr))

		targets := mgr.GetSyncTargets()
		queued := 0
		failed := []string{}

		for _, target := range targets {
			if err := mgr.TriggerReconciliation(target.Name); err != nil {
				logger.Warn("Failed to queue target for reconciliation",
					zap.String("target", target.Name),
					zap.Error(err))
				failed = append(failed, target.Name)
			} else {
				queued++
			}
		}

		w.Header().Set("Content-Type", "application/json")

		if len(failed) == 0 {
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "queued",
				"count":  queued,
			})
		} else {
			w.WriteHeader(http.StatusPartialContent)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "partial",
				"queued": queued,
				"failed": failed,
			})
		}
	})
}
