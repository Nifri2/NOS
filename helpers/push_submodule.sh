#!/bin/bash
# Stage, commit and push changes in the compiler submodule

SUBMODULE_PATH="compiler"

cd "$SUBMODULE_PATH" 2>/dev/null || { echo "Submodule not found: $SUBMODULE_PATH"; exit 1; }

# First checkout main branch (avoid detached HEAD issues)
echo "Checking out main branch..."
git checkout main 2>/dev/null || git checkout master 2>/dev/null

# Pull latest
echo "Pulling latest..."
git pull 2>/dev/null

# Check for changes
if git diff --quiet 2>/dev/null && git diff --cached --quiet 2>/dev/null; then
    echo "No changes in submodule"
    exit 0
fi

echo "Staging changes in $SUBMODULE_PATH..."
git add -A 2>/dev/null

# Get diff for commit message
DIFF=$(git diff --cached --stat 2>/dev/null)
echo "$DIFF"

# Generate commit message using Claude (haiku)
echo "Generating commit message..."
COMMIT_MSG=$(claude -p "Generate a concise git commit message for these changes. Output ONLY the commit message, nothing else. Use Conventional Commits.

Changes:
$DIFF" --model haiku --output-format text 2>/dev/null)

if [ -z "$COMMIT_MSG" ]; then
    COMMIT_MSG="Update compiler"
fi

echo "Committing: $COMMIT_MSG"
git commit -m "$COMMIT_MSG

Commit Auto-Generated" 2>/dev/null

echo "Pushing submodule..."
git push 2>/dev/null

echo "Done! ^w^"
