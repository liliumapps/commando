#!/usr/bin/env bash
# Build a fully isolated sandbox for recording commando GIFs.
#
# ISOLATION: everything commando reads is redirected into $SANDBOX —
#   HOME, XDG_CONFIG_HOME, XDG_DATA_HOME, and HISTFILE all point inside it.
# The recordings therefore CANNOT see your real cheats, memory, bookmarks, or
# shell history. Every command shown is seeded here from universal samples.
#
# Usage:  bash docs/recordings/build-sandbox.sh
# Output: a ready-to-record sandbox at docs/recordings/.sandbox
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SANDBOX="$REPO_ROOT/docs/recordings/.sandbox"

# Start clean every time so recordings are deterministic. A prior run may have
# left read-only files (e.g. a Go module cache under $HOME/go), so make the tree
# writable before removing it.
if [ -d "$SANDBOX" ]; then
  chmod -R u+w "$SANDBOX" 2>/dev/null || true
  rm -rf "$SANDBOX"
fi
mkdir -p "$SANDBOX/bin" "$SANDBOX/.config" "$SANDBOX/.local/share"

# The commando binary the recordings will run (built by record.sh).
cp "$REPO_ROOT/commando" "$SANDBOX/bin/commando"

# --- A realistic demo git project ------------------------------------------
# Gives the git cheats' `src` commands (authors, branches) real candidates,
# and gives project detection a Makefile + package.json to discover.
PROJECT="$SANDBOX/demo"
mkdir -p "$PROJECT"
cd "$PROJECT"

git init -q -b main
git config user.email "demo@example.com"
git config user.name  "Ada Lovelace"

cat > README.md <<'EOF'
# demo

A sample project used to record commando's docs GIFs.
EOF
printf 'package main\n\nfunc main() {}\n'            > main.go
printf 'package main\n\nfunc helper() {}\n'          > helper.go
git add . && git commit -q -m "Initial commit"

# A second author + more history so `git log --author` has candidates.
git config user.name "Grace Hopper"
printf 'package main\n\n// TODO: docs\nfunc doc() {}\n' > docs.go
git add docs.go && git commit -q -m "Add docs stub"
git config user.name "Ada Lovelace"

# A couple of extra branches for the branch cheats.
git branch feature/login
git branch release/1.0

# Project markers for auto-detection (make + npm targets, zero config).
cat > Makefile <<'EOF'
build:      ## Compile the binary
	go build -o app .

test:       ## Run the test suite
	go test ./...

lint:       ## Run the linter
	golangci-lint run

fmt:        ## Format the code
	gofmt -w .
EOF

cat > package.json <<'EOF'
{
  "name": "demo",
  "scripts": {
    "dev": "vite",
    "build": "vite build",
    "test": "vitest run",
    "lint": "eslint ."
  }
}
EOF

# --- A showcase cheat crafted for clean recordings --------------------------
# Universal, non-sensitive commands designed to demo each feature in one flow:
# a prefilled default, an optional [ ] segment, and a multi-select with
# candidates. Written into the sandbox's cheats dir (seeded cheats are added by
# commando itself on first run and are kept too).
mkdir -p "$SANDBOX/.config/commando/cheats"
cat > "$SANDBOX/.config/commando/cheats/demo.toml" <<'EOF'
# Showcase commands for the docs GIFs.

[[cmd]]
title = "Archive tarball"
desc = "Archive project files into a tarball."
tmpl = "tar czf <archive> [--exclude=<exclude>] <files>"
tags = ["tar", "archive", "backup"]

  [[cmd.var]]
  name    = "archive"
  default = "release.tar.gz"

  [[cmd.var]]
  name = "exclude"

  [[cmd.var]]
  name  = "files"
  src   = "ls *.go"
  multi = true
  each  = '"{}"'

[[cmd]]
title = "Create git branch"
desc = "Create and switch to a new git branch."
tmpl = "git switch -c <branch>"
tags = ["git", "branch", "new"]

  [[cmd.var]]
  name     = "branch"
  remember = "project"
EOF

# --- Seed a bit of shell history (universal, non-sensitive) ------------------
# commando reads this live; it is entirely synthetic sample data.
cat > "$SANDBOX/.zsh_history" <<'EOF'
git status
git pull --rebase
docker compose up -d
kubectl get pods -A
tar czf backup.tar.gz src
find . -name '*.go' -type f
rg 'TODO' --type go
npm run build
EOF

echo "Sandbox ready at: $SANDBOX"
echo "Demo project:     $PROJECT"
