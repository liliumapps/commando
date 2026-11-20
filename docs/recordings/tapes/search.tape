# "Search & run" — fuzzy-search across sources, pick a command, it lands on the
# prompt for review (no variables to fill).
# Render:  vhs docs/recordings/tapes/search.tape

Output docs/assets/gifs/search.gif

Require commando

Set Shell zsh
Set FontSize 24
Set Width 1440
Set Height 840
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
Sleep 400ms
Enter
Sleep 1.2s

# Fuzzy filter — a single token spans title, description, template, and tags.
Type "lint"
Sleep 1.8s
Enter
Sleep 1.5s

# Assembled command is on the prompt — pause to read.
Sleep 2.2s
