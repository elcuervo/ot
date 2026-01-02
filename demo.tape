Output demo.gif

Set WindowBar Colorful
Set BorderRadius 20

Set Shell "zsh"
Set FontSize 18
Set Width 1200
Set Height 800
Set Padding 20

# === QUICK START ===
Type "ot ./examples/vault"
Sleep 500ms
Enter
Sleep 2.5s

# Navigate smoothly
Type "j"
Sleep 250ms
Type "j"
Sleep 250ms
Type "j"
Sleep 250ms
Type "j"
Sleep 400ms

# Jump to bottom and back
Type "G"
Sleep 700ms
Type "g"
Sleep 700ms

# === TOGGLE A TASK ===
Type "jj"
Sleep 400ms
Space
Sleep 1.2s

# === INLINE EDITING ===
Type "j"
Sleep 300ms
Type "e"
Sleep 800ms

# Move to end and add tag
Ctrl+E
Type " #priority"
Sleep 500ms
Enter
Sleep 1.2s

# === ADD NEW TASK ===
Type "a"
Sleep 700ms
Type "Review quarterly goals"
Sleep 400ms
Enter
Sleep 1.2s

# === SEARCH ===
Type "/"
Sleep 400ms
Type "tutorial"
Sleep 500ms
Enter
Sleep 300ms
Space
Sleep 800ms
Escape
Sleep 600ms

# === PRIORITY ===
Type "+"
Sleep 200ms
Type "+"
Sleep 200ms
Type "+"
Sleep 600ms

# Reset priority
Type "0"
Sleep 600ms

# Set highest priority
Type "!"
Sleep 800ms

# === REFRESH ===
Type "r"
Sleep 1s

# Back to top
Type "g"
Sleep 500ms

# === QUIT ===
Type "q"
Sleep 600ms

# === GLOB PATTERNS ===
Type "ot --list './examples/project-*/todo.md'"
Sleep 500ms
Enter
Sleep 2.5s

# === INLINE QUERIES ===
Type "ot --list ./examples/vault -q 'due before 2026-01-01'"
Sleep 500ms
Enter
Sleep 2s
