// Command econome-admin is the offline recovery CLI (no-SMTP recovery paths,
// technical/05 §8). It ships in the same image (for `docker exec`) and builds
// locally as econome-admin.exe.
//
// At increment 0 the subcommands are declared but not implemented; the real
// recovery logic (reset-password, reset-2fa, user admin, backup) lands with the
// auth increments (1 and 8) against the on-volume SQLite database.
package main

import (
	"fmt"
	"os"
)

// version is overridden at build time via -ldflags (guardrails/04 §4).
var version = "dev"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Printf("econome-admin %s\n", version)
	case "reset-password", "reset-2fa", "backup", "user":
		fmt.Fprintf(os.Stderr, "econome-admin: %q is not implemented yet (lands with the auth increments)\n", os.Args[1])
		os.Exit(1)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "econome-admin: unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `econome-admin — EconoMe offline recovery CLI

Usage:
  econome-admin <command> [args]

Commands (planned):
  reset-password   Reset a user's password (forces must_change_password)
  reset-2fa        Reset a user's 2FA enrolment
  user             List / deactivate / reactivate users
  backup           Take a VACUUM INTO snapshot of the database
  version          Print the build version
  help             Show this help
`)
}
