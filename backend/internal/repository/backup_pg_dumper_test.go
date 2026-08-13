//go:build unit

package repository

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestPgDumperRestoreUsesFailFastTransactionalStdin(t *testing.T) {
	const restoreSQL = "CREATE TABLE restore_probe (id bigint);\n"

	var commandName string
	var commandArgs []string
	var command *exec.Cmd
	dumper := &PgDumper{
		cfg: &config.DatabaseConfig{
			Host:     "db.internal",
			Port:     55432,
			User:     "restore_user",
			Password: "restore_password",
			DBName:   "restore_db",
			SSLMode:  "require",
		},
		restoreCommandContext: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			commandName = name
			commandArgs = append([]string(nil), args...)
			command = exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPgDumperRestoreHelperProcess$")
			command.Env = append(os.Environ(), "GO_WANT_PG_DUMPER_HELPER=1")
			return command
		},
	}

	err := dumper.Restore(context.Background(), strings.NewReader(restoreSQL))

	require.Equal(t, "psql", commandName)
	require.Equal(t, []string{
		"-h", "db.internal",
		"-p", "55432",
		"-U", "restore_user",
		"-d", "restore_db",
		"--no-psqlrc",
		"--set=ON_ERROR_STOP=on",
		"--single-transaction",
		"--file=-",
	}, commandArgs)
	require.NotNil(t, command)
	require.Contains(t, command.Env, "PGPASSWORD=restore_password")
	require.Contains(t, command.Env, "PGSSLMODE=require")

	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 3, exitErr.ExitCode())
	require.ErrorContains(t, err, "synthetic psql script failure")
	require.ErrorContains(t, err, restoreSQL)
}

func TestPgDumperRestoreHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_PG_DUMPER_HELPER") != "1" {
		return
	}

	payload, err := io.ReadAll(os.Stdin)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "read stdin: %v", err)
		os.Exit(97)
	}
	_, _ = fmt.Fprintf(os.Stderr, "synthetic psql script failure: %s", payload)
	os.Exit(3)
}
