# "Project detection" — summon commando inside a repo and see auto-detected
# make / npm targets and git actions, with zero configuration.
# Render:  vhs docs/recordings/tapes/project.tape

Output docs/assets/gifs/project.gif

Require commando

Set Shell zsh
Set FontSize 24
Set Width 1440
Set Height 864
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
Sleep 700ms

# Show we're in a normal project with a Makefile + package.json.
Type "ls"
Sleep 300ms
Enter
Sleep 1.4s

Type "commando"
Sleep 300ms
Enter
Sleep 1.4s

# Type a project command shape — it matches the detected Makefile target without
# any config in ~/.config/commando.
Type "make build"
Sleep 2s

# Pick it; it lands on the prompt.
Enter
Sleep 2s
