package config

import (
	"fmt"
	"os"

	env "github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Log      LogConfig
	Redis    RedisConfig
	Database DatabaseConfig
	Internal InternalConfig
	Supabase SupabaseConfig
	Midtrans MidtransConfig
	FCM      FCMConfig
}

type AppConfig struct {
	Name                   string `env:"APP_NAME" envDefault:"cafeTelkom-api"`
	Env                    string `env:"APP_ENV" envDefault:"development"`
	Version                string `env:"APP_VERSION" envDefault:"dev"`
	ShutdownTimeoutSeconds int    `env:"APP_SHUTDOWN_TIMEOUT_SECONDS" envDefault:"10"`
}

type HTTPConfig struct {
	Port string `env:"APP_PORT"`
}

type LogConfig struct {
	Level string `env:"LOG_LEVEL" envDefault:"info"`
}

type RedisConfig struct {
	Addr     string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	Password string `env:"REDIS_PASSWORD"`
	DB       int    `env:"REDIS_DB" envDefault:"0"`
	Required bool   `env:"REDIS_REQUIRED" envDefault:"false"`
}

type DatabaseConfig struct {
	URL               string `env:"DATABASE_URL"`
	Host              string `env:"SUPABASE_DB_HOST" envDefault:"aws-1-ap-northeast-2.pooler.supabase.com"`
	Port              int    `env:"SUPABASE_DB_PORT" envDefault:"5432"`
	Name              string `env:"SUPABASE_DB_NAME" envDefault:"postgres"`
	User              string `env:"SUPABASE_DB_USER" envDefault:"postgres.kangzprbrstwuuejpso"`
	Password          string `env:"SUPABASE_DB_PASSWORD"`
	SSLMode           string `env:"SUPABASE_DB_SSLMODE" envDefault:"require"`
	MaxOpenConns      int32  `env:"DB_MAX_OPEN_CONNS" envDefault:"10"`
	MinIdleConns      int32  `env:"DB_MIN_IDLE_CONNS" envDefault:"2"`
	ConnMaxLifetimeM  int    `env:"DB_CONN_MAX_LIFETIME_MINUTES" envDefault:"30"`
	ConnMaxIdleTimeM  int    `env:"DB_CONN_MAX_IDLE_TIME_MINUTES" envDefault:"5"`
	HealthcheckSecond int    `env:"DB_HEALTHCHECK_SECONDS" envDefault:"5"`
	Required          bool   `env:"DB_REQUIRED" envDefault:"false"`
}

type InternalConfig struct {
	APIKey string `env:"INTERNAL_API_KEY" envDefault:"change-me"`
}

type SupabaseConfig struct {
	URL     string `env:"SUPABASE_URL"`
	AnonKey string `env:"SUPABASE_ANON_KEY"`
}

type MidtransConfig struct {
	Env         string `env:"MIDTRANS_ENV" envDefault:"sandbox"`
	ClientKey   string `env:"MIDTRANS_CLIENT_KEY"`
	ServerKey   string `env:"MIDTRANS_SERVER_KEY"`
	SnapBaseURL string `env:"MIDTRANS_SNAP_BASE_URL" envDefault:"https://app.sandbox.midtrans.com"`
	CoreBaseURL string `env:"MIDTRANS_CORE_BASE_URL" envDefault:"https://api.sandbox.midtrans.com"`
	WebhookURL  string `env:"MIDTRANS_WEBHOOK_URL"`
}

type FCMConfig struct {
	Enabled           bool   `env:"FCM_ENABLED" envDefault:"false"`
	ProjectID         string `env:"FCM_PROJECT_ID"`
	TopicNewProducts  string `env:"FCM_TOPIC_NEW_PRODUCTS" envDefault:"new-products"`
	CredentialFile    string `env:"GOOGLE_APPLICATION_CREDENTIALS"`
	BatchSize         int32  `env:"FCM_OUTBOX_BATCH_SIZE" envDefault:"10"`
	PollSeconds       int    `env:"FCM_OUTBOX_POLL_SECONDS" envDefault:"30"`
	MaxRetries        int32  `env:"FCM_OUTBOX_MAX_RETRIES" envDefault:"3"`
	RetryDelaySeconds int    `env:"FCM_OUTBOX_RETRY_DELAY_SECONDS" envDefault:"60"`
}

func Load() (Config, error) {
	// Ignore missing .env so runtime env vars still work in containers/CI.
	_ = godotenv.Load()

	cfg := Config{}
	if err := env.Parse(&cfg); err != nil {
		return Config{}, err
	}

	if cfg.HTTP.Port == "" {
		cfg.HTTP.Port = os.Getenv("PORT")
	}
	if cfg.HTTP.Port == "" {
		cfg.HTTP.Port = "8080"
	}
	if cfg.HTTP.Port == "" {
		return Config{}, fmt.Errorf("APP_PORT cannot be empty")
	}
	if cfg.Database.Required && cfg.DatabaseURL() == "" {
		return Config{}, fmt.Errorf("database connection is required when DB_REQUIRED=true")
	}

	return cfg, nil
}

func (c Config) DatabaseURL() string {
	if c.Database.URL != "" {
		return c.Database.URL
	}

	if c.Database.Password == "" {
		return ""
	}

	return fmt.Sprintf(
		"postgresql://%s:%s@%s:%d/%s?sslmode=%s",
		c.Database.User,
		c.Database.Password,
		c.Database.Host,
		c.Database.Port,
		c.Database.Name,
		c.Database.SSLMode,
	)
}
