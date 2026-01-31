// Copyright (c) 2026 Arsenii Kvachan
// SPDX-License-Identifier: MIT

// Package db provides an interface to the database
package db

import "database/sql"

// HirevecDatabase is the global database connection pool.
var HirevecDatabase *sql.DB
