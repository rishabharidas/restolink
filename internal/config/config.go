package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	DatabaseURL    string
	RedisURL       string
	JWTSecret      string
	PublicEncKey   string
	CORSOrigins    string
	RootDomain     string
	WhatsAppToken  string
	WhatsAppPhoneID string
}

func LoadConfig() *Config {
	// Try loading from .env if present
	if err := godotenv.Load(); err != nil {
		// Also try ../.env
		_ = godotenv.Load("../.env")
	}

	cfg := &Config{
		Port:           getEnv("PORT", "8080"),
		DatabaseURL:    getEnv("DATABASE_URL", "postgres://resto_admin:resto_secure_password@localhost:5432/restodb?sslmode=disable"),
		RedisURL:       getEnv("REDIS_URL", "redis://localhost:6379"),
		JWTSecret:      getEnv("JWT_SECRET", "super_secret_jwt_key_at_least_32_characters_long"),
		PublicEncKey:   getEnv("PUBLIC_ENC_KEY", "encryption_key_for_sensitive_fields_32_chars"),
		CORSOrigins:    getEnv("CORS_ORIGINS", "*"),
		RootDomain:     getEnv("ROOT_DOMAIN", "resto.local"),
		WhatsAppToken:  os.Getenv("WHATSAPP_API_TOKEN"),
		WhatsAppPhoneID: os.Getenv("WHATSAPP_PHONE_NUMBER_ID"),
	}

	log.Printf("[Config] Loaded configuration. Server Port: %s", cfg.Port)
	return cfg
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
