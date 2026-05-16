# Releasing

1. Update `Version` in `lib-flume-water.go` and `TestVersion` in `lib-flume-water_test.go`
2. Commit on a branch: `git commit -m "chore: bump version to X.Y.Z"`
3. Open a PR against `main` and merge it once CI is green
4. After the merge, on `main`:
   - `git pull origin main`
   - `git tag -a vX.Y.Z -m "Release vX.Y.Z"`
   - `git push origin vX.Y.Z`

The release workflow runs tests and creates a GitHub release via GoReleaser automatically when the tag is pushed.
