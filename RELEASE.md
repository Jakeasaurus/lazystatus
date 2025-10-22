# Release Process

This repository uses GitHub Actions to automate building, testing, and releasing lazystatus to Homebrew.

## Setup

### 1. Create a Personal Access Token (PAT)

1. Go to GitHub Settings → Developer settings → Personal access tokens → Tokens (classic)
2. Generate a new token with these permissions:
   - `repo` (full control)
   - `workflow` (if you want to trigger workflows)
3. Copy the token

### 2. Add Secret to Repository

1. Go to your lazystatus repository on GitHub
2. Settings → Secrets and variables → Actions
3. Click "New repository secret"
4. Name: `HOMEBREW_TAP_TOKEN`
5. Value: Paste your PAT
6. Click "Add secret"

## Releasing a New Version

To release a new version:

```bash
# Tag the release (replace x.y.z with version number)
git tag -a v0.3.0 -m "Release v0.3.0"

# Push the tag
git push origin v0.3.0
```

This will automatically:
1. ✅ Run tests
2. 🏗️ Build binaries for:
   - macOS (Intel & Apple Silicon)
   - Linux (AMD64 & ARM64)
3. 📦 Create release archives with checksums
4. 🚀 Create a GitHub release with auto-generated release notes
5. 🍺 Update your Homebrew tap repository

## Workflow Files

### `.github/workflows/release.yml`
- Triggered on version tags (`v*`)
- Builds cross-platform binaries
- Creates GitHub release
- Updates Homebrew tap formula

### `.github/workflows/ci.yml`
- Triggered on pushes and PRs
- Runs tests with coverage
- Checks code with `go vet`
- Builds the binary

## Manual Testing Before Release

Before pushing a release tag, test locally:

```bash
# Run tests
go test ./...

# Build for your platform
go build -o lazystatus .

# Test the binary
./lazystatus --version
./lazystatus
```

## Versioning

Follow semantic versioning (semver):
- `v1.0.0` - Major release (breaking changes)
- `v0.3.0` - Minor release (new features)
- `v0.2.1` - Patch release (bug fixes)

## Troubleshooting

### Release failed
- Check the Actions tab in GitHub for error logs
- Verify the `HOMEBREW_TAP_TOKEN` secret is set correctly
- Ensure the tag follows the `v*` pattern

### Homebrew tap not updating
- Verify the token has `repo` permissions
- Check that `jakeasaurus/homebrew-tap` repository exists
- Review the "Update Homebrew tap" job logs

## Installation for Users

Once released, users can install via:

```bash
# Add tap (first time only)
brew tap jakeasaurus/tap

# Install
brew install lazystatus

# Or install directly
brew install jakeasaurus/tap/lazystatus

# Upgrade to latest
brew upgrade lazystatus
```
