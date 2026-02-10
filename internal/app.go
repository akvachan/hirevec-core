// Package app provides a high-level interface to app modules
package app

import (
	"context"
	"fmt"
	"os/signal"
	"syscall"

	"github.com/akvachan/hirevec-backend/internal/logger"
	"github.com/akvachan/hirevec-backend/internal/server"
	"github.com/akvachan/hirevec-backend/internal/store"
	"github.com/akvachan/hirevec-backend/internal/utils"
	"github.com/akvachan/hirevec-backend/internal/vault"
)

type AppConfig struct {
	Host               string
	Port               string
	ReadTimeout        string
	WriteTimeout       string
	GracePeriod        string
	PostgresHost       string
	PostgresPort       string
	PostgresDB         string
	PostgresUser       string
	PostgresPassword   string
	LogLevel           string
	SymmetricKeyHex    string
	AsymmetricKeyHex   string
	GoogleClientID     string
	GoogleClientSecret string
	AppleClientID      string
	AppleClientSecret  string
}

func Run(c AppConfig) error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Init(
		logger.LoggerConfig{
			Level: utils.ParseLogLevelWithDefault(c.LogLevel, logger.DefaultLogLevel),
		},
	)

	vault, err := vault.NewVault(
		ctx,
		vault.VaultConfig{
			Host:               c.Host,
			Port:               c.Port,
			GoogleClientID:     c.GoogleClientID,
			GoogleClientSecret: c.GoogleClientSecret,
			AppleClientID:      c.AppleClientID,
			AppleClientSecret:  c.AppleClientSecret,
			SymmetricKeyHex:    c.SymmetricKeyHex,
			AsymmetricKeyHex:   c.AsymmetricKeyHex,
		},
	)
	if err != nil {
		return fmt.Errorf("vault init failed: %w", err)
	}

	store, err := store.NewStore(
		store.StoreConfig{
			PostgresHost:     c.PostgresHost,
			PostgresPort:     utils.ParseUint16WithDefault(c.PostgresPort, 5432),
			PostgresDB:       c.PostgresDB,
			PostgresUser:     c.PostgresUser,
			PostgresPassword: c.PostgresPassword,
		},
	)
	if err != nil {
		return fmt.Errorf("store init failed: %w", err)
	}

	return server.Run(
		ctx,
		server.ServerConfig{
			Host:         c.Host,
			Port:         utils.ParseUint16WithDefault(c.Port, 8080),
			ReadTimeout:  utils.ParseTimeWithDefault(c.ReadTimeout, server.DefaultReadTimeout),
			WriteTimeout: utils.ParseTimeWithDefault(c.WriteTimeout, server.DefaultWriteTimeout),
			GracePeriod:  utils.ParseTimeWithDefault(c.GracePeriod, server.DefaultGracePeriod),
		},
		store,
		vault,
	)
}
