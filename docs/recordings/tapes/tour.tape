# Global overview tour — the whole flow in one take: fuzzy search across
# sources → fill-in form (prefilled default, skipped optional, multi-select) →
# command preview → assembled command on the prompt.
# Render:  vhs docs/recordings/tapes/tour.tape

Output docs/assets/gifs/tour.gif

Require commando

Set Shell zsh
Set FontSize 24
Set Width 1440
Set Height 912
Set Padding 24
Set Framerate 24
Set TypingSpeed 65ms
Set Theme "GitHub Dark"

Hide
Type "source docs/recordings/env.sh"
Enter
Show

Ctrl+L
Sleep 300ms
Sleep 800ms

# Summon commando — one launcher over cheats, project commands, and history.
Type "commando"
Sleep 350ms
Enter
Sleep 1.3s

# Fuzzy-search across every source at once.
Type "tarball"
Sleep 1.6s
Enter
Sleep 1.2s

# Field 1: archive — a sensible default is prefilled. Keep it.
Sleep 1.1s
Enter
Sleep 800ms

# Field 2: exclude — optional. Skip it and the whole [--exclude=...] drops out.
Ctrl+S
Sleep 1.3s
Enter
Sleep 800ms

# Field 3: files — multi-select. Toggle two candidates; preview updates live.
Type " "
Sleep 800ms
Up
Sleep 500ms
Type " "
Sleep 1.4s
Enter
Sleep 1.5s

# The finished command lands on the prompt — review, edit, run.
Sleep 2.5s
