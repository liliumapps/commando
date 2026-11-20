# "Optional + multi-select" — the signature feature. Keep a prefilled default,
# skip an optional [ ] segment (it vanishes), multi-select files with Space,
# and watch the command preview assemble.
# Uses the showcase cheat:  tar czf <archive> [--exclude=<exclude>] <files>
# Render:  vhs docs/recordings/tapes/optional-multi.tape

Output docs/assets/gifs/optional-multi.gif

Require commando

Set Shell zsh
Set FontSize 24
Set Width 1440
Set Height 912
Set Padding 24
Set Framerate 24
Set TypingSpeed 70ms
Set Theme "GitHub Dark"

Hide
Type "source docs/recordings/env.sh"
Enter
Show

Ctrl+L
Sleep 300ms
Sleep 800ms
Type "commando"
Sleep 300ms
Enter
Sleep 1.2s

# Find the showcase cheat (its title and description include "tarball").
Type "tarball"
Sleep 1.5s
Enter
Sleep 1.2s

# Field 1: archive — a default (release.tar.gz) is prefilled. Keep it.
Sleep 1.2s
Enter
Sleep 900ms

# Field 2: exclude — optional. Skip it; the whole [--exclude=...] fragment drops.
Ctrl+S
Sleep 1.4s
Enter
Sleep 900ms

# Field 3: files — multi-select with candidates (main.go, helper.go, docs.go).
# Candidates are drawn bottom-up, so Up moves to the next item. Toggle two
# with Space; the command preview updates as we go.
Type " "
Sleep 900ms
Up
Sleep 500ms
Type " "
Sleep 1.4s
Enter
Sleep 1.5s

# Assembled command on the prompt.
Sleep 2.2s
