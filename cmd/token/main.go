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
	"slices"
	"syscall"
	"time"

	"github.com/akvachan/hirevec-backend/cmd/common"
	"github.com/akvachan/hirevec-backend/internal"
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
	store, err := hirevec.NewPostgresStore(storeCfg)
	if err != nil {
		die("failed to create a new store", "err", err)
	}

	userID, roles, err := store.GetUserByProvider(hirevec.ProviderGoogle, "admin")
	switch {
	case errors.Is(err, hirevec.ErrUserNotFound):
		slog.Info("provisioning an admin")

		admin := hirevec.User{
			Provider:       hirevec.ProviderGoogle,
			ProviderUserID: "admin",
			Email:          "admin@admin.com",
			FullName:       "admin",
			UserName:       "admin",
		}

		userID, err = store.CreateUser(admin)
		if err != nil {
			die("failed to create an admin", "err", err)
		}

		store.CreateCandidate(hirevec.Candidate{
			UserID: userID,
			About:  "Admin Candidate",
		})

		store.CreateRecruiter(hirevec.Recruiter{
			UserID: userID,
		})

	case errors.Is(err, hirevec.ErrUserNoRole):
		slog.Info("admin does not have a role, creating candidate and recruiter for the admin", "userID", userID)

		if err := store.CreateCandidate(hirevec.Candidate{
			UserID: userID,
			About:  "Admin Candidate",
		}); err != nil {
			die("failed to create a candidate for the admin", "userID", userID, "err", err)
		}

		if err := store.CreateRecruiter(hirevec.Recruiter{
			UserID: userID,
		}); err != nil {
			die("failed to create a recruiter for the admin", "userID", userID, "err", err)
		}

	case !slices.Contains(roles, "recruiter"):
		slog.Info("admin does not have a recruiter role, creating recruiter for the admin", "userID", userID)
		if err := store.CreateRecruiter(hirevec.Recruiter{
			UserID: userID,
		}); err != nil {
			die("failed to create a recruiter for the admin", "userID", userID, "err", err)
		}

	case !slices.Contains(roles, "candidate"):
		slog.Info("admin does not have a candidate role, creating candidate for the admin", "userID", userID)
		if err := store.CreateCandidate(hirevec.Candidate{
			UserID: userID,
			About:  "Admin Candidate",
		}); err != nil {
			die("failed to create a candidate for the admin", "userID", userID, "err", err)
		}

	case err != nil:
		die("failed to get an admin", "err", err)

	}

	slog.Info("creating recommendations for admin user as candidate")

	adminCandidate, err := store.GetCandidateByUserID(userID)
	if err != nil {
		die("failed to get admin candidate", "err", err)
	}

	positions, err := store.GetPositions(hirevec.Pagination{Limit: 100})
	if err != nil {
		die("failed to list positions for admin candidate", "err", err)
	}

	for _, pos := range positions.Items {
		recID, err := store.CreateRecommendation(pos.ID, adminCandidate.ID)
		if err != nil {
			if errors.Is(err, hirevec.ErrRecommendationExists) {
				slog.Info("recommendation already exists for admin candidate", "position_id", pos.ID)
				continue
			}
			die("failed to create recommendation for admin candidate", "positionID", pos.ID, "err", err)
		}
		slog.Info("created recommendation for admin candidate", "position_id", pos.ID, "recommendation_id", recID)
	}

	vaultCfg := hirevec.VaultConfig{
		SymmetricKeyHex:       os.Getenv("SYMMETRIC_KEY"),
		AsymmetricKeyHex:      os.Getenv("ASYMMETRIC_KEY"),
		AccessTokenExpiration: 365 * 24 * time.Hour, // 1 year
	}
	vault, err := hirevec.NewPasetoVault(ctx, vaultCfg)
	if err != nil {
		die("failed to create a new vault", "err", err)
	}

	token, err := vault.CreateAccessToken(userID, hirevec.ProviderGoogle.Raw(), hirevec.ScopeType{hirevec.ScopeValueTypeAdmin})
	if err != nil {
		die("failed to create a refresh token", "err", err)
	}

	fmt.Printf("\n%s\n", token.AccessToken)
}

func die(msg string, args ...any) {
	slog.Error(msg, args...)
	os.Exit(1)
}
