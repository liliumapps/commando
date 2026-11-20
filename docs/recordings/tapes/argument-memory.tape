# "Argument memory" — run a command once, then summon again: the value you
# entered is remembered and floated to the top as a candidate (frecency).
# Uses the showcase cheat:  git switch -c <branch>   (remember = "project")
# Render:  vhs docs/recordings/tapes/argument-memory.tape

Output docs/assets/gifs/argument-memory.gif

Require commando

Set Shell zsh
Set FontSize 24
Set Width 1440
Set Height 864
Set Padding 24
Set Framerate 24
Set TypingSpeed 75ms
Set Theme "GitHub Dark"

Hide
Type "source docs/recordings/env.sh"
Enter
Show

Ctrl+L
Sleep 300ms
Sleep 700ms

# --- First run: type a branch name so commando remembers it. ---
Type "commando"
Sleep 300ms
Enter
Sleep 1.2s
Type "switch"
Sleep 1.2s
Enter
Sleep 1s

Type "feature/payments"
Sleep 1.4s
Enter
Sleep 1.5s

# The command is on the prompt — clear the line, we just wanted the value stored.
Ctrl+U
Sleep 800ms

# --- Second run: the remembered value is now a top candidate. ---
Type "commando"
Sleep 300ms
Enter
Sleep 1.2s
Type "switch"
Sleep 1.2s
Enter
Sleep 1.5s

# The branch field now offers "feature/payments" as a remembered candidate,
# most-recent first — no retyping.
Sleep 2.5s
