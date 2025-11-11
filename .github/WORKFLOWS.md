# GitHub Actions Workflows

This repository uses GitHub Actions for continuous integration and automated releases.

## Workflows

### 1. Test Workflow (`test.yml`)

**Triggers:**
- Push to `main`, `master`, or `develop` branches
- Pull requests targeting `main`, `master`, or `develop` branches

**What it does:**
- Runs tests
- Executes `go vet` for code analysis
- Runs tests with race detection and generates coverage reports
- Uploads coverage to Codecov (if `CODECOV_TOKEN` is configured)
- Runs golangci-lint for code quality checks

**Required secrets:**
- `CODECOV_TOKEN` (optional) - For uploading coverage reports to Codecov

### 2. Release Workflow (`release.yml`)

**Triggers:**
- Push of tags matching pattern `v*.*.*` (e.g., `v0.1.0`, `v1.2.3`)

**What it does:**
- Runs all tests to verify the release
- Uses GoReleaser to create GitHub releases
- Generates release notes with changelog
- Creates source archives
- Generates checksums

**Required permissions:**
- `contents: write` - To create releases
- `packages: write` - For publishing packages

## Creating a Release

### Step 1: Update the version

Update the version constant in `lib-flume-water.go`:

```go
const (
    // Version is the current version of the library
    Version = "0.2.0" // Update this
    ...
)
```

Update the version test in `lib-flume-water_test.go`:

```go
// Current version should be 0.2.0
if Version != "0.2.0" {
    t.Errorf("Version = %s, want 0.2.0", Version)
}
```

### Step 2: Commit and push changes

```bash
git add lib-flume-water.go lib-flume-water_test.go
git commit -m "chore: bump version to 0.2.0"
git push origin main
```

### Step 3: Create and push a tag

```bash
# Create an annotated tag
git tag -a v0.2.0 -m "Release v0.2.0"

# Push the tag to trigger the release workflow
git push origin v0.2.0
```

### Step 4: Monitor the release

1. Go to the Actions tab in GitHub
2. Watch the "Release" workflow run
3. Once complete, check the Releases page for the new release

## Release Versioning

This project follows [Semantic Versioning](https://semver.org/):

- **MAJOR** version (X.0.0) - Incompatible API changes
- **MINOR** version (0.X.0) - New functionality (backwards compatible)
- **PATCH** version (0.0.X) - Bug fixes (backwards compatible)

## GoReleaser Configuration

The release process is configured in `.goreleaser.yaml`. Key features:

- **Library-focused**: Skips binary builds since this is a library
- **Source archives**: Creates `.tar.gz` files with source code
- **Changelog generation**: Automatically categorizes commits by type
- **Checksums**: Generates SHA-256 checksums for all artifacts
- **GitHub integration**: Creates releases with formatted notes

## Changelog Commit Conventions

To have commits automatically categorized in the changelog, use conventional commit messages:

- `feat:` - New features
- `fix:` - Bug fixes
- `perf:` - Performance improvements
- `refactor:` - Code refactoring
- `deps:` - Dependency updates
- `docs:` - Documentation (excluded from changelog)
- `test:` - Tests (excluded from changelog)
- `chore:` - Maintenance (excluded from changelog)

Example:
```bash
git commit -m "feat: add support for real-time flow rate queries"
git commit -m "fix: handle null values in device battery level"
```

## Troubleshooting

### Release workflow fails

**Issue**: GoReleaser fails with permission errors
**Solution**: Ensure the repository has "Read and write permissions" enabled:
1. Go to Settings → Actions → General
2. Under "Workflow permissions", select "Read and write permissions"
3. Save the changes

**Issue**: Test workflow fails on go vet
**Solution**: Run `go vet ./...` locally to identify and fix issues before pushing

### Coverage reports not uploading

**Issue**: Codecov upload fails
**Solution**:
1. Add `CODECOV_TOKEN` to repository secrets
2. Or set `fail_ci_if_error: false` to make it optional (already configured)

## Local Testing

### Test the build locally

```bash
go test -v -race -coverprofile=coverage.out ./...
```

### Validate GoReleaser configuration

```bash
# Install goreleaser
go install github.com/goreleaser/goreleaser@latest

# Test the configuration without publishing
goreleaser release --snapshot --clean
```

This will create a local `dist/` directory with the release artifacts.
