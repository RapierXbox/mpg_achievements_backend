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

	qrActionRepo := repository.NewQRActionRepo(session)
	qrCodeRepo := repository.NewQRCodeRepo(session)
	userQrScanRepo := repository.NewUserQRScanRepo(session)

	// initialize services (logic)
	accountService := service.NewAccountService(accountRepo, sessionRepo, cfg.PepperSecret)
	sessionService := service.NewSessionService(sessionRepo, cfg.PepperSecret, time.Hour*24*time.Duration(cfg.RefreshTokenTTL))
	qrService := service.NewQRService(qrActionRepo, qrCodeRepo, userQrScanRepo)

	// initialize handlers (http parsing)
	authHandler := handlers.NewAuthHandler(accountService, sessionService, cfg)
	accountHandler := handlers.NewAccountHandler(accountService)

	qrCodeHandler := handlers.NewQRCodeHandler(qrService)
	qrCodeManagementHandler := handlers.NewQRCodeManagementHandler(qrService, accountRepo)

	debugHandler := handlers.NewDebugHandler(cfg)

	// public routes
	router.HandleFunc("/auth/register", accountHandler.Register).Methods("POST")
	router.HandleFunc("/auth/login", authHandler.Login).Methods("POST")
	router.HandleFunc("/auth/refresh", authHandler.SilentLogin).Methods("POST")

	// authenticated routes (use auth middleware)
	authRouter := router.PathPrefix("/").Subrouter()
	authRouter.Use(middleware.AuthMiddleware(cfg, sessionService))

	authRouter.HandleFunc("/auth/change_password", accountHandler.ChangePassword).Methods("POST")
	authRouter.HandleFunc("/auth/logout", authHandler.Logout).Methods("POST")
	authRouter.HandleFunc("/auth/delete_account", accountHandler.Delete).Methods("POST")

	authRouter.HandleFunc("/qr/scan", qrCodeHandler.GetQRAction).Methods("GET")

	authRouter.HandleFunc("/qr-mgmt/add_action", qrCodeManagementHandler.AddQRAction).Methods("POST")
	authRouter.HandleFunc("/qr-mgmt/add_code", qrCodeManagementHandler.AddQRCode).Methods("POST")
	authRouter.HandleFunc("/qr-mgmt/delete_code", qrCodeManagementHandler.DeleteQRCode).Methods("POST")
	authRouter.HandleFunc("/qr-mgmt/delete_action", qrCodeManagementHandler.DeleteQRAction).Methods("POST")
	authRouter.HandleFunc("/qr-mgmt/list_codes", qrCodeManagementHandler.GetAllQRCodes).Methods("GET")
	authRouter.HandleFunc("/qr-mgmt/list_actions", qrCodeManagementHandler.GetAllQRActions).Methods("GET")

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
