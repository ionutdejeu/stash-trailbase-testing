package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/urfave/cli/v3"
)

type copilotMCPConfig struct {
	MCPServers map[string]copilotMCPServer `json:"mcpServers"`
}

type copilotMCPServer struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Tools   []string          `json:"tools,omitempty"`
}

func mcpCopilotConfigCmd(ctx context.Context, cmd *cli.Command) error {
	_ = ctx

	serverName := strings.TrimSpace(cmd.String("name"))
	if serverName == "" {
		return fmt.Errorf("server name cannot be empty")
	}

	projectRoot := cmd.String("project-root")
	if projectRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("resolve working directory: %w", err)
		}
		projectRoot = cwd
	}

	projectRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}

	if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err != nil {
		return fmt.Errorf("project root must contain go.mod: %w", err)
	}

	configPath := cmd.String("config")
	if configPath == "" {
		configPath = ".env"
	}
	if !filepath.IsAbs(configPath) {
		configPath = filepath.Join(projectRoot, configPath)
	}

	command, args := copilotLaunchCommand(projectRoot, cmd.Bool("with-consolidation"))

	config := copilotMCPConfig{
		MCPServers: map[string]copilotMCPServer{
			serverName: {
				Type:    "local",
				Command: command,
				Args:    args,
				Env: map[string]string{
					"STASHCONFIG": configPath,
				},
				Tools: []string{"*"},
			},
		},
	}

	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	fmt.Println(string(b))
	return nil
}

func copilotLaunchCommand(projectRoot string, withConsolidation bool) (string, []string) {
	args := []string{"run", "./cmd/cli", "mcp", "execute"}
	if withConsolidation {
		args = append(args, "--with-consolidation")
	}

	if runtime.GOOS == "windows" {
		script := "Set-Location -LiteralPath " + quotePowerShell(projectRoot) + "; go " + strings.Join(args, " ")
		return "powershell", []string{"-NoLogo", "-NoProfile", "-Command", script}
	}

	script := "cd " + quotePOSIX(projectRoot) + " && go " + strings.Join(args, " ")
	return "sh", []string{"-lc", script}
}

func quotePowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func quotePOSIX(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
