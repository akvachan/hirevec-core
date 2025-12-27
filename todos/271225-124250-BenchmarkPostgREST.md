---
date: "2025-12-27"
tags:
status: Active
---

# 271225-124250-BenchmarkPostgREST

## Description

We need a comprehensive benchmark of PostgREST against our data and custom handler logic that exists right now.

Tests should be implemented in a systematic manner.

First measure raw performance of each handler as-is, then setup PostgREST on our PGSQL database and measure it's raw performance for the same queries.
