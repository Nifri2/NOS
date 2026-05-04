#!/bin/bash
# Generate a commit message using Claude, commit, and push

# Don't exit on profile errors
set -o pipefail

# Check for uncommitted changes
if git diff --quiet 2>/dev/null && git diff --cached --quiet 2>/dev/null; then
    echo "No changes to commit"
    exit 0
fi

# Stage all changes
echo "Staging changes..."
git add -A 2>/dev/null

# Get the diff for context
DIFF=$(git diff --cached --stat 2>/dev/null)
echo "Changes to commit:"
echo "$DIFF"
echo ""

# Generate commit message using Claude (haiku for speed and cost)
echo "Generating commit message..."
COMMIT_MSG=$(claude -p "Generate a concise git commit message for these changes. Output ONLY the commit message, nothing else. No quotes, no explanation, just the message itself (can be multiple lines for body). Use Conventional Commits.

Changes:
$DIFF" --model haiku --output-format text 2>/dev/null)

if [ -z "$COMMIT_MSG" ]; then
    echo "Failed to generate commit message"
    exit 1
fi

echo "Commit message:"
echo "---"
echo "$COMMIT_MSG"
echo "---"
echo ""

# Commit with the generated message
echo "Committing..."
git commit -m "$COMMIT_MSG

Commit Auto-Generated" 2>/dev/null

# Push to origin
echo "Pushing to origin..."
git push 2>/dev/null

echo "Done! ^w^"
