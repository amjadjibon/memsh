#!/usr/bin/env bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Default values
DRY_RUN=false
TAG=""

# Help function
show_help() {
    cat << EOF
memsh Release Script

Usage: $0 [OPTIONS]

Options:
    --tag TAG         Version tag for the release (e.g., v1.0.0) [REQUIRED]
    --dry-run         Run in dry-run mode (no git pushes, no release)
    -h, --help        Show this help message

Examples:
    $0 --tag v1.0.0              # Create full release
    $0 --tag v1.0.0 --dry-run    # Test release without pushing

What this script does:
  • Validates working directory state
  • Commits and pushes any uncommitted changes
  • Cleans dist/ and build artifacts
  • Creates and pushes git tag
  • Runs goreleaser (builds, signs, creates release, updates homebrew tap)
  • Updates homebrew-memsh submodule reference

EOF
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --tag)
            TAG="$2"
            shift 2
            ;;
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            show_help
            exit 1
            ;;
    esac
done

# Validate TAG is provided
if [ -z "$TAG" ]; then
    echo -e "${RED}Error: --tag parameter is required${NC}"
    echo "Usage: $0 --tag VERSION [--dry-run]"
    echo "Example: $0 --tag v1.0.0"
    exit 1
fi

# Validate TAG format (starts with v)
if [[ ! "$TAG" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo -e "${RED}Error: Invalid tag format. Expected format: vX.Y.Z (e.g., v1.0.0)${NC}"
    exit 1
fi

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}memsh Release Script${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""
echo -e "${BLUE}Configuration:${NC}"
echo "  Tag: $TAG"
echo "  Dry Run: $DRY_RUN"
echo ""

# Check if we're in the right directory
if [ ! -f "go.mod" ] || [ ! -d ".git" ]; then
    echo -e "${RED}Error: Must be run from the memsh repository root${NC}"
    exit 1
fi

# Check for uncommitted changes
echo -e "${BLUE}→ Checking for uncommitted changes...${NC}"
if [ -n "$(git status --porcelain)" ]; then
    echo -e "${YELLOW}Found uncommitted changes. Committing and pushing...${NC}"

    if [ "$DRY_RUN" = false ]; then
        git add -A
        git commit -m "chore: prepare for release $TAG"
        echo -e "${BLUE}→ Pushing changes to origin...${NC}"
        git pull origin main --rebase
        git push origin main
        echo -e "${GREEN}✅ Changes pushed to repository${NC}"
    else
        echo -e "${YELLOW}[DRY-RUN] Would commit and push changes${NC}"
    fi
else
    echo -e "${GREEN}✓ Working directory is clean${NC}"
fi

# Clean dist directory
echo ""
echo -e "${BLUE}→ Cleaning dist directory...${NC}"
if [ "$DRY_RUN" = false ]; then
    make clean > /dev/null 2>&1 || true
    rm -rf dist/
    echo -e "${GREEN}✓ Dist directory cleaned${NC}"
else
    echo -e "${YELLOW}[DRY-RUN] Would clean dist/ directory${NC}"
fi

# Check if tag already exists
echo ""
echo -e "${BLUE}→ Checking if tag already exists...${NC}"
if git rev-parse "$TAG" >/dev/null 2>&1; then
    echo -e "${RED}Error: Tag $TAG already exists${NC}"
    echo "To delete: git tag -d $TAG && git push --delete origin $TAG"
    exit 1
fi
echo -e "${GREEN}✓ Tag $TAG is available${NC}"

# Create and push tag
echo ""
echo -e "${BLUE}→ Creating git tag $TAG...${NC}"
if [ "$DRY_RUN" = false ]; then
    git tag -a "$TAG" -m "Release $TAG"
    echo -e "${GREEN}✓ Tag created${NC}"

    echo -e "${BLUE}→ Pushing tag to origin...${NC}"
    git push origin "$TAG"
    echo -e "${GREEN}✓ Tag pushed${NC}"
else
    echo -e "${YELLOW}[DRY-RUN] Would create and push tag: $TAG${NC}"
fi

# Run goreleaser
echo ""
echo -e "${BLUE}→ Running goreleaser...${NC}"
if [ "$DRY_RUN" = false ]; then
    goreleaser release --clean
    echo -e "${GREEN}✓ Goreleaser completed${NC}"
else
    echo -e "${YELLOW}[DRY-RUN] Would run: goreleaser release --clean${NC}"
    echo -e "${YELLOW}[DRY-RUN] Running in test mode...${NC}"
    goreleaser release --skip=publish,validate --clean
fi

# Manually push cask to homebrew-memsh repository
echo ""
echo -e "${BLUE}→ Pushing cask to homebrew-memsh repository...${NC}"
if [ "$DRY_RUN" = false ]; then
    if [ -f "dist/homebrew/Casks/memsh.rb" ]; then
        if [ -d "homebrew-memsh" ] && [ -f "homebrew-memsh/.git" ]; then
            # Create Casks directory if it doesn't exist
            mkdir -p homebrew-memsh/Casks

            # Copy the generated cask
            cp dist/homebrew/Casks/memsh.rb homebrew-memsh/Casks/

            # Go into homebrew-memsh directory
            cd homebrew-memsh

            # Fetch and reset to avoid any divergent branches
            echo -e "${BLUE}  → Syncing with remote...${NC}"
            git fetch origin main
            git reset --hard origin/main

            # Add and commit the cask
            echo -e "${BLUE}  → Committing cask...${NC}"
            git add Casks/memsh.rb

            if [ -n "$(git status --porcelain Casks/memsh.rb)" ]; then
                git commit -m "Update memsh cask to $TAG"
                echo -e "${BLUE}  → Pushing to homebrew-memsh...${NC}"
                git push origin main
                echo -e "${GREEN}✓ Cask pushed to homebrew-memsh${NC}"
            else
                echo -e "${YELLOW}ℹ️  Cask already up to date${NC}"
            fi

            # Go back to main repo
            cd ..

            # Update submodule reference in main repo
            echo -e "${BLUE}→ Updating submodule reference in main repo...${NC}"
            git add homebrew-memsh
            if [ -n "$(git status --porcelain homebrew-memsh)" ]; then
                git commit --amend --no-edit --no-verify
                git push origin main --force-with-lease
                echo -e "${GREEN}✓ Submodule reference updated${NC}"
            fi
        else
            echo -e "${YELLOW}⚠️  homebrew-memsh submodule not found, skipping cask push${NC}"
        fi
    else
        echo -e "${YELLOW}⚠️  Cask file not found at dist/homebrew/Casks/memsh.rb${NC}"
    fi
else
    echo -e "${YELLOW}[DRY-RUN] Would push cask to homebrew-memsh${NC}"
fi

# Summary
echo ""
echo -e "${BLUE}========================================${NC}"
if [ "$DRY_RUN" = true ]; then
    echo -e "${YELLOW}✨ DRY-RUN COMPLETE${NC}"
    echo ""
    echo -e "${BLUE}What would happen on full release:${NC}"
    echo "  • Uncommitted changes would be committed and pushed"
    echo "  • Git tag $TAG would be created and pushed"
    echo "  • GitHub release would be published"
    echo "  • Goreleaser would automatically:"
    echo "    - Build binaries for all platforms"
    echo "    - Sign macOS binaries"
    echo "    - Push cask to homebrew-memsh repository"
    echo "  • homebrew-memsh submodule reference would be updated"
    echo ""
    echo -e "${YELLOW}Generated artifacts:${NC}"
    echo "  📄 Cask: dist/homebrew/Casks/memsh.rb"
    echo "  📦 Binaries: dist/*.tar.gz"
else
    echo -e "${GREEN}✅ RELEASE $TAG CREATED SUCCESSFULLY!${NC}"
    echo ""
    echo -e "${BLUE}Release URLs:${NC}"
    echo "  📦 GitHub: https://github.com/amjadjibon/memsh/releases/tag/$TAG"
    echo "  🍺 Homebrew: https://github.com/amjadjibon/homebrew-memsh"
    echo ""
    echo -e "${BLUE}Installation:${NC}"
    echo "  brew tap amjadjibon/memsh"
    echo "  brew install memsh"
fi
echo -e "${BLUE}========================================${NC}"
