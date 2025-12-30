Output demo.gif

Set WindowBar Colorful
Set BorderRadius 20

Set Shell "zsh"
Set FontSize 18
Set Width 1200
Set Height 800
Set Padding 20

# Start the app with vault and query file
Type "./ot --vault ./examples/vault ./examples/query.md"
Sleep 500ms
Enter
Sleep 1.5s

# === NAVIGATION ===
# Navigate through sections using arrow keys
Down 3
Sleep 400ms
Down
Sleep 400ms
Down
Sleep 400ms

# Navigate using vim keys (j/k)
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "j"
Sleep 300ms
Type "k"
Sleep 300ms
Type "k"
Sleep 500ms

# === TOGGLING TASKS ===
# Toggle a task with space
Space
Sleep 800ms

# Move down and toggle another
Down 2
Sleep 400ms
Space
Sleep 800ms

# === JUMP NAVIGATION ===
# Jump to top with 'g'
Type "g"
Sleep 600ms

# Jump to bottom with 'G'
Type "G"
Sleep 600ms

# Back to middle
Up 5
Sleep 400ms

# === SEARCH FEATURE ===
# Enter search mode with '/'
Type "/"
Sleep 600ms

# Type search query
Type "report"
Sleep 800ms

# Navigate search results with Enter
Enter
Sleep 500ms

# Move through results
Down
Sleep 400ms
Up
Sleep 400ms

# Toggle task in search mode
Space
Sleep 700ms

# Exit search with Escape
Escape
Sleep 600ms

# === SEARCH AGAIN ===
# Search for tasks by category
Type "/"
Sleep 400ms
Type "groceries"
Sleep 600ms
Enter
Sleep 500ms
Space
Sleep 700ms
Escape
Sleep 600ms

# === REFRESH ===
# Refresh the view with 'r'
Type "r"
Sleep 1s

# Final navigation to show updated state
Type "g"
Sleep 400ms
Down 3
Sleep 300ms
Down 2
Sleep 500ms

# === QUIT ===
# Exit the app
Type "q"
Sleep 300ms
