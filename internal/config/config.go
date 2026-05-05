package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	App    App    `mapstructure:"app"`
	HTTP   HTTP   `mapstructure:"http"`
	DB     DB     `mapstructure:"db"`
	JWT    JWT    `mapstructure:"jwt"`
	Logger Logger `mapstructure:"logger"`
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

	v.SetDefault("db.dsn", "")
	v.SetDefault("db.max_open_conns", 25)
	v.SetDefault("db.max_idle_conns", 5)
	v.SetDefault("db.conn_max_lifetime", "30m")

	v.SetDefault("jwt.secret", "")
	v.SetDefault("jwt.issuer", "qooim")
	v.SetDefault("jwt.expires_in", "24h")

	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.format", "json")
}
