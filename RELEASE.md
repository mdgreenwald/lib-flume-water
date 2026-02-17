# Releasing

1. Update `Version` in `lib-flume-water.go` and `TestVersion` in `lib-flume-water_test.go`
2. Commit: `git commit -m "chore: bump version to X.Y.Z"`
3. Tag: `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
4. Push: `git push origin main --tags`

The release workflow runs tests and creates a GitHub release via GoReleaser automatically.
