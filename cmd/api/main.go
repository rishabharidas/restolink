package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/rishabharidas/restolink/internal/config"
	"github.com/rishabharidas/restolink/internal/database"
	"github.com/rishabharidas/restolink/internal/handlers"
	"github.com/rishabharidas/restolink/internal/middleware"
	"github.com/rishabharidas/restolink/internal/realtime"
)

func main() {
	cfg := config.LoadConfig()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Connect to PostgreSQL
	pgDB, err := database.NewPostgresDB(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("[FATAL] Could not initialize PostgreSQL: %v", err)
	}
	defer pgDB.Close()

	// 2. Connect to Redis (optional/graceful fallback if local Redis is down)
	var redisClient *database.RedisClient
	if cfg.RedisURL != "" {
		redisClient, _ = database.NewRedisClient(cfg.RedisURL)
		if redisClient != nil {
			defer redisClient.Close()
		}
	}

	// 3. Start Real-time WebSocket Hub
	hub := realtime.NewHub()
	go hub.Run()

	// 4. Initialize Handlers
	tenantHandler := handlers.NewTenantHandler()
	menuHandler := handlers.NewMenuHandler(pgDB.Pool, hub)
	orderHandler := handlers.NewOrderHandler(pgDB.Pool, hub)

	var deliveryHandler *handlers.DeliveryHandler
	var tableHandler *handlers.TableHandler
	if redisClient != nil {
		deliveryHandler = handlers.NewDeliveryHandler(pgDB.Pool, redisClient.Client, hub)
		tableHandler = handlers.NewTableHandler(pgDB.Pool, redisClient.Client, hub)
	} else {
		deliveryHandler = handlers.NewDeliveryHandler(pgDB.Pool, nil, hub)
		tableHandler = handlers.NewTableHandler(pgDB.Pool, nil, hub)
	}

	authHandler := handlers.NewAuthHandler(pgDB.Pool, cfg.JWTSecret)
	cmsHandler := handlers.NewCMSHandler(pgDB.Pool)
	reservationHandler := handlers.NewReservationHandler(pgDB.Pool, hub)
	wsHandler := handlers.NewWSHandler(hub)

	// 5. Router & Middleware
	r := chi.NewRouter()

	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(middleware.NewCORSMiddleware(cfg.CORSOrigins))
	r.Use(middleware.TenantMiddleware(pgDB.Pool, cfg.RootDomain))

	// Health Check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","service":"restolink-go","timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `"}`))
	})

	// WebSocket Endpoint
	r.Get("/ws", wsHandler.HandleConnection)

	// API v1 Routes
	r.Route("/api/v1", func(api chi.Router) {
		// Tenant Endpoints
		api.Get("/tenant/config", tenantHandler.GetConfig)

		// Storefront CMS Endpoints
		api.Get("/cms/pages/{slug}", cmsHandler.GetPage)
		api.Put("/cms/landing-mode", cmsHandler.UpdateLandingMode)
		api.Post("/cms/pages/{slug}/sections", cmsHandler.AddSection)
		api.Put("/cms/sections/{id}", cmsHandler.UpdateSection)
		api.Delete("/cms/sections/{id}", cmsHandler.DeleteSection)
		api.Put("/cms/sections/reorder", cmsHandler.ReorderSections)

		// Menu Management Endpoints
		api.Get("/menu", menuHandler.GetMenu)
		api.Post("/menu/categories", menuHandler.CreateCategory)
		api.Put("/menu/categories/{id}", menuHandler.UpdateCategory)
		api.Delete("/menu/categories/{id}", menuHandler.DeleteCategory)
		api.Post("/menu/items", menuHandler.CreateMenuItem)
		api.Put("/menu/items/{id}", menuHandler.UpdateMenuItem)
		api.Delete("/menu/items/{id}", menuHandler.DeleteMenuItem)
		api.Put("/menu/items/{id}/outlets/{outlet_id}", menuHandler.UpdateOutletPricing)
		api.Post("/menu/import-csv", menuHandler.ImportCSV)
		api.Patch("/menu/items/{id}/availability", menuHandler.ToggleAvailability)
		api.Get("/table/qr/{token}", menuHandler.ResolveTableQR)

		// Waiter & Table Operations
		api.Get("/tables", tableHandler.ListTables)
		api.Post("/tables/transfer", tableHandler.TransferTable)
		api.Post("/tables/merge", tableHandler.MergeTables)
		api.Post("/tables/{id}/items", tableHandler.AppendItemsToOrder)
		api.Post("/table/{id}/bell", tableHandler.RingBell)
		api.Get("/table/{id}/cart", tableHandler.GetTableCart)
		api.Post("/table/{id}/cart", tableHandler.SyncTableCart)
		api.Post("/table/generate-qr", tableHandler.GenerateTableQR)

		// Reservations
		api.Post("/reservations", reservationHandler.CreateReservation)
		api.Get("/reservations", reservationHandler.ListReservations)

		// Authentication
		api.Post("/auth/login", authHandler.Login)
		api.Post("/auth/logout", authHandler.Logout)

		// Orders & Pipeline
		api.Get("/orders", orderHandler.ListOrders)
		api.Post("/orders", orderHandler.CreateOrder)
		api.Get("/orders/{id}", orderHandler.GetOrder)
		api.Patch("/orders/{id}/status", orderHandler.UpdateStatus)
		api.Post("/orders/{id}/cancel", orderHandler.CancelOrder)
		api.Patch("/orders/{order_id}/items/{item_id}/status", orderHandler.UpdateOrderItemStatus)

		// Delivery & Live Tracking
		api.Post("/delivery/location", deliveryHandler.IngestLocation)
		api.Get("/delivery/track/{order_id}", deliveryHandler.GetTracking)
	})

	// 6. Start HTTP Server with Graceful Shutdown
	server := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("🚀 Restolink API Server listening on port %s", cfg.Port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[FATAL] ListenAndServe error: %v", err)
		}
	}()

	// Graceful shutdown listener
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	log.Println("Shutting down Restolink API server gracefully...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[ERROR] Server shutdown error: %v", err)
	}
	log.Println("Restolink API server stopped.")
}
