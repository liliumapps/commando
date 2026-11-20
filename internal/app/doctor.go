package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/liliumapps/commando/internal/config"
	"github.com/liliumapps/commando/internal/persist"
	"github.com/liliumapps/commando/internal/provider/history"
	"github.com/liliumapps/commando/internal/provider/project"
	"github.com/liliumapps/commando/internal/sources"
	"github.com/liliumapps/commando/internal/state"
)

// CheckStatus is the severity of one doctor check.
type CheckStatus string

const (
	CheckOK      CheckStatus = "ok"
	CheckWarning CheckStatus = "warn"
	CheckError   CheckStatus = "error"
)

// Check is one actionable environment or configuration diagnostic.
type Check struct {
	Name   string
	Status CheckStatus
	Detail string
}

// Doctor inspects configuration, state, history, providers, shell integration,
// and binary discovery without requiring the full application to open.
func Doctor() []Check {
	var checks []Check
	add := func(name string, status CheckStatus, detail string) {
		checks = append(checks, Check{Name: name, Status: status, Detail: detail})
	}

	paths, err := config.DefaultPaths()
	if err != nil {
		add("paths", CheckError, err.Error())
		return checks
	}
	if err := paths.EnsureDirs(); err != nil {
		add("paths", CheckError, err.Error())
		return checks
	}
	add("paths", CheckOK, paths.ConfigDir+"; "+paths.DataDir)

	cfg, err := config.Load(paths)
	if err != nil {
		add("config", CheckError, err.Error())
		return checks
	}
	configPath := filepath.Join(paths.ConfigDir, "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		add("config", CheckOK, "using defaults; "+configPath+" is optional")
	} else {
		add("config", CheckOK, configPath)
	}

	cheats, cheatErr := config.LoadCheats(paths)
	if cheatErr != nil {
		add("cheats", CheckWarning, fmt.Sprintf("%d valid; %v", len(cheats), cheatErr))
	} else {
		add("cheats", CheckOK, fmt.Sprintf("%d command(s) loaded", len(cheats)))
	}
	add(sourcesCheck(paths))

	// Opening state also moves a corrupt memory/usage file aside; the
	// "recovered" check below then lists it.
	if _, err := state.Open(paths.DataDir); err != nil {
		add("state", CheckError, err.Error())
	} else {
		add("state", CheckOK, paths.DataDir)
	}
	// Report corrupt state/bookmark files moved aside by this or an earlier
	// run, so the user can inspect or delete them.
	var quarantined []string
	for _, p := range []string{
		filepath.Join(paths.DataDir, "memory.toml"),
		filepath.Join(paths.DataDir, "usage.toml"),
		paths.BookmarksPath,
	} {
		quarantined = append(quarantined, persist.QuarantinedFiles(p)...)
	}
	if len(quarantined) > 0 {
		add("recovered", CheckWarning, fmt.Sprintf("corrupt file(s) were moved aside and replaced with empty state: %s", strings.Join(quarantined, ", ")))
	}

	historyPath := history.New().Path()
	if historyPath == "" {
		add("history", CheckWarning, "no history file detected; set HISTFILE")
	} else {
		info, err := os.Stat(historyPath)
		if err != nil {
			add("history", CheckWarning, err.Error())
		} else {
			add("history", CheckOK, fmt.Sprintf("%s (%d bytes)", historyPath, info.Size()))
		}
	}

	projectEntries, projectErr := project.New(cfg.Providers).Entries(buildContext())
	if projectErr != nil {
		add("providers", CheckWarning, projectErr.Error())
	} else {
		add("providers", CheckOK, fmt.Sprintf("%d project command(s) detected", len(projectEntries)))
	}

	shellName := filepath.Base(os.Getenv("SHELL"))
	switch shellName {
	case "zsh", "bash", "fish":
		add("shell", CheckOK, shellName+" integration supported")
	default:
		add("shell", CheckWarning, fmt.Sprintf("%s integration is not supported; use zsh, bash, or fish", fallback(shellName, "unknown shell")))
	}

	if path, err := exec.LookPath("commando"); err != nil {
		add("binary", CheckWarning, "commando is not on PATH")
	} else {
		add("binary", CheckOK, path)
	}
	return checks
}

// sourcesCheck reports the cheat sources: how many, how many commands they
// load, and any missing clone, unreviewed commit, or load error.
func sourcesCheck(paths config.Paths) (name string, status CheckStatus, detail string) {
	count, commands, problems := sources.Status(paths)
	switch {
	case len(problems) > 0:
		msgs := make([]string, len(problems))
		for i, p := range problems {
			msgs[i] = strings.ReplaceAll(p.Error(), "\n", "; ")
		}
		return "sources", CheckWarning, fmt.Sprintf("%d source(s), %d command(s) loaded; %s", count, commands, strings.Join(msgs, "; "))
	case count == 0:
		return "sources", CheckOK, "none; add one with `commando cheats add <git-url> [file]`"
	default:
		return "sources", CheckOK, fmt.Sprintf("%d source(s), %d command(s) loaded", count, commands)
	}
}

func fallback(value, defaultValue string) string {
	if strings.TrimSpace(value) == "" {
		return defaultValue
	}
	return value
}
