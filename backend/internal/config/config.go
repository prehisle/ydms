package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds application level configuration.
type Config struct {
	HTTPPort int
	NDR      NDRConfig
	Auth     AuthConfig
	Debug    DebugConfig
	DB       DBConfig
	JWT      JWTConfig
	Admin    AdminBootstrapConfig
	Prefect  PrefectConfig
	MinIO    MinIOConfig
}

// NDRConfig stores settings for the upstream NDR service.
type NDRConfig struct {
	BaseURL string
	APIKey  string
}

// DebugConfig stores flags that affect logging and diagnostics.
type DebugConfig struct {
	Traffic bool
}

// AuthConfig contains defaults for user/request metadata.
type AuthConfig struct {
	DefaultUserID string
	AdminKey      string
}

// DBConfig stores database connection settings.
type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// AdminBootstrapConfig stores default admin bootstrap credentials.
type AdminBootstrapConfig struct {
	Username    string
	Password    string
	DisplayName string
}

// JWTConfig stores JWT authentication settings.
type JWTConfig struct {
	Secret string
	Expiry string // e.g., "24h", "7d"
}

// PrefectConfig stores Prefect integration settings.
type PrefectConfig struct {
	BaseURL       string // Prefect Server API URL (empty to disable Prefect integration)
	WebhookSecret string // Secret for callback webhook verification
	Timeout       int    // Request timeout in seconds
	PublicBaseURL string // Public URL for callbacks (defaults to http://localhost:{port})
}

// MinIOConfig stores MinIO proxy settings for static assets.
type MinIOConfig struct {
	URL string // MinIO server URL (empty to disable proxy)
}

// Load builds a Config object from environment variables, providing sane defaults.
func Load() Config {
	return Config{
		HTTPPort: parseEnvInt("YDMS_HTTP_PORT", 9180),
		NDR: NDRConfig{
			BaseURL: firstNonEmpty(os.Getenv("YDMS_NDR_BASE_URL"), "not_set"),
			APIKey:  firstNonEmpty(os.Getenv("YDMS_NDR_API_KEY"), "not_set"),
		},
		Auth: AuthConfig{
			DefaultUserID: firstNonEmpty(os.Getenv("YDMS_DEFAULT_USER_ID"), "dms"),
			AdminKey:      firstNonEmpty(os.Getenv("YDMS_ADMIN_KEY"), "not_set"),
		},
		Debug: DebugConfig{
			Traffic: parseEnvBool("YDMS_DEBUG_TRAFFIC", false),
		},
		DB: DBConfig{
			Host:     firstNonEmpty(os.Getenv("YDMS_DB_HOST"), "localhost"),
			Port:     parseEnvInt("YDMS_DB_PORT", 5432),
			User:     firstNonEmpty(os.Getenv("YDMS_DB_USER"), "postgres"),
			Password: firstNonEmpty(os.Getenv("YDMS_DB_PASSWORD"), ""),
			DBName:   firstNonEmpty(os.Getenv("YDMS_DB_NAME"), "ydms"),
			SSLMode:  firstNonEmpty(os.Getenv("YDMS_DB_SSLMODE"), "disable"),
		},
		JWT: JWTConfig{
			Secret: firstNonEmpty(os.Getenv("YDMS_JWT_SECRET"), "change-me-in-production"),
			Expiry: firstNonEmpty(os.Getenv("YDMS_JWT_EXPIRY"), "24h"),
		},
		Admin: AdminBootstrapConfig{
			Username:    firstNonEmpty(os.Getenv("YDMS_DEFAULT_ADMIN_USERNAME"), "super_admin"),
			Password:    firstNonEmpty(os.Getenv("YDMS_DEFAULT_ADMIN_PASSWORD"), "admin123456"),
			DisplayName: firstNonEmpty(os.Getenv("YDMS_DEFAULT_ADMIN_DISPLAY_NAME"), "超级管理员"),
		},
		Prefect: PrefectConfig{
			BaseURL:       os.Getenv("YDMS_PREFECT_BASE_URL"), // Empty by default (disabled)
			WebhookSecret: os.Getenv("YDMS_PREFECT_WEBHOOK_SECRET"),
			Timeout:       parseEnvInt("YDMS_PREFECT_TIMEOUT", 300),
			PublicBaseURL: os.Getenv("YDMS_PUBLIC_BASE_URL"), // For callback URLs
		},
		MinIO: MinIOConfig{
			URL: os.Getenv("YDMS_MINIO_URL"), // Empty by default (disabled)
		},
	}
}

// HTTPAddress formats the address string the HTTP server should bind to.
func (c Config) HTTPAddress() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

func parseEnvInt(key string, defaultValue int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return defaultValue
	}

	var value int
	if _, err := fmt.Sscanf(raw, "%d", &value); err != nil {
		return defaultValue
	}
	return value
}

func parseEnvBool(key string, defaultValue bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if raw == "" {
		return defaultValue
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return defaultValue
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
