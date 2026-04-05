// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/akvachan/hirevec-core"
	"github.com/akvachan/hirevec-core/cmd/common"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := common.Loadenv(".env"); err != nil {
		slog.Warn("failed to load .env, using system environment", "err", err)
	}
	pgHost := common.Getenv("POSTGRES_HOST", "localhost")
	pgPort := common.Getenv("POSTGRES_PORT", "5432")
	pgDB := common.Getenv("POSTGRES_DB", "hirevec")
	pgUser := os.Getenv("POSTGRES_USER")
	pgPassword := os.Getenv("POSTGRES_PASSWORD")
	pgPortParsed := hirevec.ParseUint16WithDefault(pgPort, 5432)

	storeCfg := hirevec.StoreConfig{
		PostgresHost:     pgHost,
		PostgresPort:     pgPortParsed,
		PostgresDB:       pgDB,
		PostgresUser:     pgUser,
		PostgresPassword: pgPassword,
	}
	store, err := hirevec.NewStore(storeCfg)
	if err != nil {
		common.Exit("failed to create a new store", "err", err)
	}

	userID, roles, err := store.GetUserByProvider(hirevec.ProviderGoogle, "google-test-001")
	if errors.Is(err, hirevec.ErrUserNoRole) {
		common.Exit("user does not have any role yet, add role in init.sql")
	}
	if err != nil {
		common.Exit("could not get userID and roles", "err", err)
	}

	vaultCfg := hirevec.VaultConfig{
		SymmetricKeyHex:       os.Getenv("SYMMETRIC_KEY"),
		AsymmetricKeyHex:      os.Getenv("ASYMMETRIC_KEY"),
		AccessTokenExpiration: 365 * 24 * time.Hour, // 1 year
	}
	vault, err := hirevec.NewVault(ctx, vaultCfg)
	if err != nil {
		common.Exit("failed to create a new vault", "err", err)
	}

	token, err := vault.CreateAccessToken(userID, hirevec.ProviderGoogle, roles)
	if err != nil {
		common.Exit("failed to create a refresh token", "err", err)
	}

	if err := os.Setenv("ACCESS_TOKEN", token.AccessToken); err != nil {
		common.Exit("failed to set ACCESS_TOKEN environment variable")
	}

	fmt.Printf("\n%s\n", token.AccessToken)
}
