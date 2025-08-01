package api

import (
	"backend/internal/api/handlers"
	"backend/internal/api/middleware"
	"backend/internal/repository"
	"backend/internal/service"
	"backend/pkg/config"

	"log"
	"net/http"
	"time"

	"github.com/gocql/gocql"
	"github.com/gorilla/mux"
)

// SetupRouter configures all application routes
func SetupRouter(session *gocql.Session, cfg *config.Config, logger *log.Logger) *mux.Router {
	router := mux.NewRouter()

	// initialize repositories (database access)
	accountRepo := repository.NewAccountRepo(session)
	sessionRepo := repository.NewSessionRepo(session)

	// initialize services (logic)
	accountService := service.NewAccountService(accountRepo, cfg.PepperSecret)
	sessionService := service.NewSessionService(sessionRepo, cfg.PepperSecret, time.Hour*24*time.Duration(cfg.RefreshTokenTTL))

	// initialize handlers (http parsing)
	authHandler := handlers.NewAuthHandler(accountService, sessionService, cfg)
	accountHandler := handlers.NewAccountHandler(accountService)

	debugHandler := handlers.NewDebugHandler(cfg)

	// public routes
	router.HandleFunc("/register", accountHandler.Register).Methods("POST")
	router.HandleFunc("/login", authHandler.Login).Methods("POST")
	router.HandleFunc("/refresh", authHandler.SilentLogin).Methods("POST")

	// authenticated routes (use auth middleware)
	authRouter := router.PathPrefix("/").Subrouter()
	authRouter.Use(middleware.AuthMiddleware(cfg, sessionService))

	authRouter.HandleFunc("/change-password", accountHandler.ChangePassword).Methods("POST")
	authRouter.HandleFunc("/logout", authHandler.Logout).Methods("POST")
	authRouter.HandleFunc("/delete", accountHandler.Delete).Methods("POST")

	authRouter.HandleFunc("/debug", debugHandler.AuthDebug).Methods("GET")

	// health check endpoint
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}).Methods("GET")

	// logging middleware
	router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
			next.ServeHTTP(w, r)
		})
	})

	return router
}
