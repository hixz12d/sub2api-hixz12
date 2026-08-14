// openai-affinity-migrate previews or applies one CAS-protected durable OpenAI
// affinity migration. Preview is the default and is read-only. Apply requires
// the exact digest from a previously saved plan file.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	_ "github.com/lib/pq"
)

func main() {
	databaseURL := flag.String("database-url", strings.TrimSpace(os.Getenv("DATABASE_URL")), "PostgreSQL URL (or DATABASE_URL)")
	fromID := flag.Int64("from-account", 0, "source OpenAI OAuth account ID")
	toID := flag.Int64("to-account", 0, "target OpenAI OAuth account ID")
	reason := flag.String("reason", "", "explicit operator reason")
	includeExpired := flag.Bool("include-expired", false, "include expired session bindings in preview")
	planOut := flag.String("plan-out", "", "write read-only preview JSON to this file")
	planIn := flag.String("plan-in", "", "read an existing preview JSON for apply")
	apply := flag.Bool("apply", false, "apply exactly one saved plan")
	confirm := flag.String("confirm", "", "exact SHA-256 plan digest required by -apply")
	flag.Parse()

	if strings.TrimSpace(*databaseURL) == "" {
		fatalf("database URL is required")
	}
	db, err := sql.Open("postgres", *databaseURL)
	if err != nil {
		fatalf("open database: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		fatalf("ping database: %v", err)
	}

	repo := repository.NewOpenAIAffinityRepository(db)
	tool := service.NewOpenAIAffinityMigrationTool(repo)
	if *apply {
		if strings.TrimSpace(*planIn) == "" || strings.TrimSpace(*confirm) == "" {
			fatalf("-apply requires -plan-in and -confirm")
		}
		plan := readPlan(*planIn)
		validateTarget(ctx, db, plan.ToAccountID)
		if err := tool.Apply(ctx, plan, *confirm); err != nil {
			fatalf("apply migration: %v", err)
		}
		fmt.Printf("applied binding migration plan digest=%s from_account=%d to_account=%d\n", plan.Digest, plan.FromAccountID, plan.ToAccountID)
		return
	}

	if *fromID <= 0 || *toID <= 0 || strings.TrimSpace(*reason) == "" || strings.TrimSpace(*planIn) != "" {
		fatalf("preview requires -from-account, -to-account, -reason and does not accept -plan-in")
	}
	validateTarget(ctx, db, *toID)
	plan, err := tool.Preview(ctx, *fromID, *toID, *reason, *includeExpired)
	if err != nil {
		fatalf("preview migration: %v", err)
	}
	payload, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		fatalf("encode plan: %v", err)
	}
	payload = append(payload, '\n')
	if strings.TrimSpace(*planOut) != "" {
		if err := os.WriteFile(*planOut, payload, 0o600); err != nil {
			fatalf("write plan: %v", err)
		}
	}
	_, _ = os.Stdout.Write(payload)
}

func readPlan(path string) *service.OpenAIAffinityMigrationPlan {
	payload, err := os.ReadFile(path)
	if err != nil {
		fatalf("read plan: %v", err)
	}
	var plan service.OpenAIAffinityMigrationPlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		fatalf("parse plan: %v", err)
	}
	return &plan
}

func validateTarget(ctx context.Context, db *sql.DB, accountID int64) {
	var platform, accountType, status string
	var schedulable bool
	if err := db.QueryRowContext(ctx, `SELECT platform,type,status,schedulable FROM accounts WHERE id=$1`, accountID).
		Scan(&platform, &accountType, &status, &schedulable); err != nil {
		fatalf("load migration target account %d: %v", accountID, err)
	}
	if platform != service.PlatformOpenAI || accountType != service.AccountTypeOAuth || status != service.StatusActive || !schedulable {
		fatalf("migration target account %d must be active, schedulable OpenAI OAuth", accountID)
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
