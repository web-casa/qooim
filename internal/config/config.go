package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App     App     `mapstructure:"app"`
	HTTP    HTTP    `mapstructure:"http"`
	DB      DB      `mapstructure:"db"`
	JWT     JWT     `mapstructure:"jwt"`
	Logger  Logger  `mapstructure:"logger"`
	Storage Storage `mapstructure:"storage"`
	AI      AI      `mapstructure:"ai"`
}

type App struct {
	Name    string `mapstructure:"name"`
	Env     string `mapstructure:"env"`
	Version string `mapstructure:"version"`
}

type HTTP struct {
	Addr            string        `mapstructure:"addr"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	APIPrefix       string        `mapstructure:"api_prefix"`
	// WebRoot points at the static SPA bundle. Empty disables static
	// serving (API-only mode).
	WebRoot string `mapstructure:"web_root"`
	// TrustedProxies is the CIDR list passed to gin's
	// SetTrustedProxies. Without it gin trusts ALL forwarders, which
	// means a malicious client can spoof X-Forwarded-For and bypass
	// per-IP rate limiting + poison server-side IP logging.
	// Default (empty) disables proxy trust entirely — c.ClientIP()
	// returns the direct peer. Set to ["127.0.0.1/32","10.0.0.0/8"]
	// or similar when running behind nginx/cloudflare.
	TrustedProxies []string `mapstructure:"trusted_proxies"`
}

type DB struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

type JWT struct {
	Secret    string        `mapstructure:"secret"`
	Issuer    string        `mapstructure:"issuer"`
	ExpiresIn time.Duration `mapstructure:"expires_in"`
}

type Logger struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

type Storage struct {
	// Backend is "local" for now (s3 lands in P3+).
	Backend string `mapstructure:"backend"`
	// LocalRoot is where the local backend writes files. Relative paths
	// are resolved against the working directory.
	LocalRoot string `mapstructure:"local_root"`
	// MaxUploadBytes caps a single upload (multipart form). 0 = no cap.
	MaxUploadBytes int64 `mapstructure:"max_upload_bytes"`
}

type AI struct {
	// Enabled gates the /api/ai/* routes. When false the handlers return
	// 404 so the existence of the feature isn't leaked.
	Enabled bool `mapstructure:"enabled"`
	// Provider is "siliconflow" today; an OpenAI-compatible endpoint
	// can also be used by setting Provider="openai" + BaseURL.
	Provider string `mapstructure:"provider"`
	BaseURL  string `mapstructure:"base_url"`
	Token    string `mapstructure:"token"`
	Model    string `mapstructure:"model"`
	// HTTPTimeout is per-request, not the streaming total — SSE streams
	// keep the connection open until the provider closes it.
	HTTPTimeout time.Duration `mapstructure:"http_timeout"`
}

func Load(path string) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetEnvPrefix("QOOIM")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %s: %w", path, err)
		}
	}

	var c Config
	if err := v.Unmarshal(&c); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return &c, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.name", "Qoo.IM")
	v.SetDefault("app.env", "dev")
	v.SetDefault("app.version", "0.0.0")

	v.SetDefault("http.addr", ":8080")
	v.SetDefault("http.read_timeout", "15s")
	v.SetDefault("http.write_timeout", "60s")
	v.SetDefault("http.shutdown_timeout", "10s")
	v.SetDefault("http.api_prefix", "/api")
	v.SetDefault("http.web_root", "./web/dist")
	v.SetDefault("http.trusted_proxies", []string{})

	v.SetDefault("db.dsn", "")
	v.SetDefault("db.max_open_conns", 25)
	v.SetDefault("db.max_idle_conns", 5)
	v.SetDefault("db.conn_max_lifetime", "30m")

	v.SetDefault("jwt.secret", "")
	v.SetDefault("jwt.issuer", "qooim")
	v.SetDefault("jwt.expires_in", "24h")

	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.format", "json")

	v.SetDefault("storage.backend", "local")
	v.SetDefault("storage.local_root", "./storage")
	v.SetDefault("storage.max_upload_bytes", int64(64*1024*1024))

	v.SetDefault("ai.enabled", false)
	v.SetDefault("ai.provider", "siliconflow")
	v.SetDefault("ai.base_url", "https://api.siliconflow.cn")
	v.SetDefault("ai.token", "")
	v.SetDefault("ai.model", "deepseek-chat")
	v.SetDefault("ai.http_timeout", "30s")
}
