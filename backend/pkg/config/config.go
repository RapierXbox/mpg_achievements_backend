package config

import (
	"os"
	"strconv"
)

// holds all application configuration parameters
type Config struct {
	ScyllaHost      string
	JWTSecret       string
	PepperSecret    string
	AccessTokenTTL  int // token lifespan in minutes
	RefreshTokenTTL int // token lifespan in days
}

// load configuration from environment variables
func Load() *Config {
	return &Config{
		ScyllaHost:      getEnv("SCYLLA_HOST", "scylladb"),
		JWTSecret:       getEnv("JWT_SECRET", ""),
		PepperSecret:    getEnv("PEPPER_SECRET", ""),
		AccessTokenTTL:  getEnvAsInt("ACCESS_TOKEN_TTL", 15),
		RefreshTokenTTL: getEnvAsInt("REFRESH_TOKEN_TTL", 365),
	}
}

// get environment variable with default
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// get integer environment variables
func getEnvAsInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}
