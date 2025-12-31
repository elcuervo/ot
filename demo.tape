Output demo.gif

Set WindowBar Colorful
Set BorderRadius 20

Set Shell "zsh"
Set FontSize 18
Set Width 1200
Set Height 800
Set Padding 20

# Start the app
Type "ot --vault ./examples/vault ./examples/query.md"
Sleep 500ms
Enter
Sleep 2s

# === NAVIGATION ===
# Scroll through the task list
Down
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms

# Vim-style navigation
Type "jj"
Sleep 500ms
Type "kk"
Sleep 500ms

# Jump to end and back
Type "G"
Sleep 600ms
Type "g"
Sleep 600ms

# === COMPLETE A TASK ===
# Navigate to "Morning standup"
Down
Sleep 300ms

# Mark it done with space
Space
Sleep 1s

# === EDIT A TASK ===
# Navigate to "Call dentist"
Down
Sleep 300ms
Down
Sleep 300ms

# Open edit modal
Type "e"
Sleep 800ms

# Add priority tag
Ctrl+E
Sleep 200ms
Type " #urgent"
Sleep 500ms

# Save
Enter
Sleep 1s

# === ADD A TASK ===
# Add a new task after current one
Type "a"
Sleep 800ms

# Type the new task
Type "Follow up on PR review"
Sleep 600ms

# Save
Enter
Sleep 1s

# === DELETE A TASK ===
# Navigate down to a task
Down
Sleep 300ms

# Show delete confirmation
Type "d"
Sleep 1.2s

# Cancel with 'n'
Type "n"
Sleep 600ms

# === SEARCH ===
# Open search
Type "/"
Sleep 500ms

# Search for tasks with links
Type "Go"
Sleep 800ms

# Navigate results
Enter
Sleep 500ms

# Toggle from search
Space
Sleep 800ms

# Exit search
Escape
Sleep 600ms

# === REFRESH ===
Type "r"
Sleep 1s

# Back to top
Type "g"
Sleep 800ms

# === QUIT ===
Type "q"
Sleep 500ms
