#!/bin/bash
# Generate a commit message using Claude, commit, and push

set -e

# Check for uncommitted changes
if git diff --quiet && git diff --cached --quiet; then
    echo "No changes to commit"
    exit 0
fi

# Stage all changes
echo "Staging changes..."
git add -A

# Get the diff for context
DIFF=$(git diff --cached --stat)
echo "Changes to commit:"
echo "$DIFF"
echo ""

# Generate commit message using Claude
echo "Generating commit message..."
COMMIT_MSG=$(claude -p "Generate a concise git commit message for these changes. Output ONLY the commit message, nothing else. No quotes, no explanation, just the message itself (can be multiple lines for body).

Changes:
$DIFF" --output-format text 2>/dev/null)

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

Auto-Generated: Claude <noreply@anthropic.com>"

# Push to origin
echo "Pushing to origin..."
git push

echo "Done! ^w^"
