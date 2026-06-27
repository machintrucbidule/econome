// Command econome-admin is the offline recovery CLI (no-SMTP recovery paths,
// technical/05 §8). It ships in the same image (for `docker exec` in prod) and
// builds locally as econome-admin.exe. It opens the on-volume SQLite database
// directly and drives the same services as the UI, so the last-admin rule and
// the password policy are enforced identically.
package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"econome/internal/config"
	"econome/internal/domain"
	"econome/internal/repo"
	"econome/internal/services"
	"econome/migrations"
)

// version is overridden at build time via -ldflags (guardrails/04 §4); the
// default mirrors the app's current semver baseline.
var version = "0.0.1"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "version", "--version", "-v":
		fmt.Printf("econome-admin %s\n", version)
		return
	case "help", "--help", "-h":
		usage()
		return
	}

	if err := dispatch(cmd, args); err != nil {
		fmt.Fprintf(os.Stderr, "econome-admin: %v\n", err)
		os.Exit(1)
	}
}

func dispatch(cmd string, args []string) error {
	svc, db, dataDir, err := open()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	ctx := context.Background()

	switch cmd {
	case "reset-password":
		return resetPassword(ctx, svc, args)
	case "reset-2fa":
		return resetTOTP(ctx, svc, args)
	case "user":
		return userCmd(ctx, svc, args)
	case "backup":
		return backupCmd(ctx, db, dataDir)
	default:
		usage()
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// open loads config, opens + migrates the on-volume database, and wires a service.
func open() (*services.Service, *sql.DB, string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, "", err
	}
	if err := cfg.EnsureDataDir(); err != nil {
		return nil, nil, "", err
	}
	secret, err := cfg.EnsureSecret()
	if err != nil {
		return nil, nil, "", err
	}
	db, err := repo.Open(filepath.Join(cfg.DataDir, "econome.db"))
	if err != nil {
		return nil, nil, "", err
	}
	if err := repo.Migrate(context.Background(), db, migrations.FS, filepath.Join(cfg.DataDir, "backups")); err != nil {
		_ = db.Close()
		return nil, nil, "", fmt.Errorf("migrations: %w", err)
	}
	store := repo.New(db)
	svc := services.New(services.Deps{
		Users: store.Users, Sessions: store.Sessions, Settings: store.Settings,
		Accounts: store.Accounts, Categories: store.Categories, Envelopes: store.Envelopes,
		Allocations: store.Allocations, Transactions: store.Transactions, Snapshots: store.Snapshots,
		NetworthMonths: store.NetworthMonths, Periods: store.Periods, PeriodEvents: store.PeriodEvents,
		Labels: store.Labels, UIPreferences: store.UIPreferences,
		Invitations: store.Invitations, TOTPBackups: store.TOTPBackups,
		Tx: store, Secret: secret,
	})
	return svc, db, cfg.DataDir, nil
}

func resetPassword(ctx context.Context, svc *services.Service, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: econome-admin reset-password <email>")
	}
	u, err := svc.FindUserByEmail(ctx, args[0])
	if err != nil {
		return userErr(args[0], err)
	}
	temp, err := svc.GenerateTempPassword()
	if err != nil {
		return err
	}
	if err := svc.AdminResetPassword(ctx, u.ID, temp); err != nil {
		return err
	}
	fmt.Printf("Temporary password for %s: %s\n", u.Email, temp)
	fmt.Println("The user must change it on next login. Transmit it out-of-band.")
	return nil
}

func resetTOTP(ctx context.Context, svc *services.Service, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: econome-admin reset-2fa <email>")
	}
	u, err := svc.FindUserByEmail(ctx, args[0])
	if err != nil {
		return userErr(args[0], err)
	}
	if err := svc.AdminResetTOTP(ctx, u.ID); err != nil {
		return err
	}
	fmt.Printf("2FA disabled for %s. They can log in with their password and re-enrol.\n", u.Email)
	return nil
}

func userCmd(ctx context.Context, svc *services.Service, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: econome-admin user <list|deactivate|reactivate> [email]")
	}
	switch args[0] {
	case "list":
		users, err := svc.ListUsers(ctx)
		if err != nil {
			return err
		}
		for _, u := range users {
			role := "member"
			if u.IsAdmin {
				role = "admin"
			}
			fmt.Printf("%-30s %-7s %-12s 2fa=%v\n", u.Email, role, u.Status, u.TOTPEnabled)
		}
		return nil
	case "deactivate", "reactivate":
		if len(args) != 2 {
			return fmt.Errorf("usage: econome-admin user %s <email>", args[0])
		}
		u, err := svc.FindUserByEmail(ctx, args[1])
		if err != nil {
			return userErr(args[1], err)
		}
		if args[0] == "deactivate" {
			if err := svc.DeactivateUser(ctx, u.ID); err != nil {
				if errors.Is(err, services.ErrLastAdmin) {
					return errors.New("cannot deactivate the last active administrator")
				}
				return err
			}
			fmt.Printf("%s deactivated; their sessions were revoked.\n", u.Email)
			return nil
		}
		if err := svc.ReactivateUser(ctx, u.ID); err != nil {
			return err
		}
		fmt.Printf("%s reactivated.\n", u.Email)
		return nil
	default:
		return fmt.Errorf("unknown user subcommand %q", args[0])
	}
}

func backupCmd(ctx context.Context, db *sql.DB, dataDir string) error {
	path, err := repo.BackupTo(ctx, db, filepath.Join(dataDir, "backups"))
	if err != nil {
		return err
	}
	fmt.Printf("Backup written to %s\n", path)
	return nil
}

func userErr(email string, err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("no account with email %q", email)
	}
	return err
}

func usage() {
	fmt.Fprint(os.Stderr, `econome-admin — EconoMe offline recovery CLI

Usage:
  econome-admin <command> [args]

Commands:
  reset-password <email>   Set a temporary password (forces a change on next login)
  reset-2fa <email>        Disable a user's 2FA so they can re-enrol
  user list                List all accounts (email, role, status, 2FA)
  user deactivate <email>  Deactivate an account (revokes its sessions)
  user reactivate <email>  Reactivate a deactivated account
  backup                   Write a VACUUM INTO snapshot under <data>/backups
  version                  Print the build version
  help                     Show this help
`)
}
