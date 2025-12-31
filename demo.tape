Output demo.gif

Set WindowBar Colorful
Set BorderRadius 20

Set Shell "zsh"
Set FontSize 18
Set Width 1200
Set Height 800
Set Padding 20

# Start the app with vault and query file
Type "ot --vault ./examples/vault ./examples/query.md"
Sleep 500ms
Enter
Sleep 1.5s

# === NAVIGATION ===
# Navigate down through tasks with arrow keys
Down
Sleep 300ms
Down
Sleep 300ms
Down
Sleep 300ms

# Navigate using vim keys (j/k)
Type "jj"
Sleep 400ms
Type "kk"
Sleep 400ms

# === JUMP NAVIGATION ===
# Jump to bottom with 'G'
Type "G"
Sleep 500ms

# Jump to top with 'g'
Type "g"
Sleep 500ms

# === TOGGLING TASKS ===
# Toggle a task with space (marks as done with date)
Down
Sleep 300ms
Space
Sleep 700ms

# Move and toggle another task
Down 2
Sleep 300ms
Space
Sleep 700ms

# === INLINE EDITING ===
# Navigate to a task
Down
Sleep 300ms

# Press 'e' to open edit popup
Type "e"
Sleep 700ms

# Edit the description (add prefix)
Ctrl+A
Sleep 200ms
Type "[URGENT] "
Sleep 500ms

# Save with Enter
Enter
Sleep 800ms

# === DELETE TASK ===
# Navigate to a task to delete
Down 2
Sleep 300ms

# Press 'd' to open delete confirmation
Type "d"
Sleep 1200ms

# Cancel deletion with 'n'
Type "n"
Sleep 500ms

# Try delete again and confirm
Type "d"
Sleep 1200ms

# Confirm deletion with 'y'
Type "y"
Sleep 800ms

# === SEARCH FEATURE ===
# Enter search mode with '/'
Type "/"
Sleep 500ms

# Search for "report"
Type "report"
Sleep 600ms

# Press Enter to navigate results
Enter
Sleep 400ms

# Navigate through search results
Down
Sleep 300ms
Up
Sleep 300ms

# Toggle a task from search results
Space
Sleep 600ms

# Edit from search results
Type "e"
Sleep 600ms

# Add tag to task
Ctrl+E
Sleep 200ms
Type " #done"
Sleep 400ms
Enter
Sleep 600ms

# Delete from search results
Down
Sleep 300ms
Type "d"
Sleep 600ms
Type "y"
Sleep 600ms

# Exit search with Escape
Escape
Sleep 500ms

# === REFRESH ===
# Refresh the view with 'r'
Type "r"
Sleep 800ms

# Show final state
Type "g"
Sleep 500ms

# === QUIT ===
Type "q"
Sleep 300ms
