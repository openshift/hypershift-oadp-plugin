# Dependencies Integration Tests

This directory contains tests that validate critical project dependencies are up-to-date and compatible.

## Tests

### `TestWatchedDependenciesAreUpToDate`

**Purpose**: Validates that all watched dependencies are up-to-date with their respective upstream main branches.

**Why this matters**: Outdated dependencies can cause scheme compatibility issues, API incompatibilities, and other runtime failures during backup/restore operations.

**How it works**:
1. Maintains a configurable map of `module -> repository URL` for watched dependencies
2. For each dependency, compares the commit hash in our pseudo-version against the latest commit in upstream main
3. Fails if any dependency is not at the latest commit
4. Provides detailed information about what needs to be updated

**Configuration**:
Dependencies are configured in the `watchedDependencies` map in `dependencies_test.go`:
```go
var watchedDependencies = map[string]string{
    "github.com/openshift/hypershift/api": "https://github.com/openshift/hypershift",
    // Add more dependencies here as needed
}
```

**What it checks**:
1. Reads current versions from `go.mod`
2. Extracts commit hashes from pseudo-versions
3. Fetches latest commit hashes from upstream repositories
4. Compares and reports any mismatches

## Running the Tests

```bash
# Run only dependency tests
go test ./tests/integration/dependencies -v

# Run with specific test
go test ./tests/integration/dependencies -v -run TestWatchedDependenciesAreUpToDate

# Example output
=== RUN   TestWatchedDependenciesAreUpToDate
=== RUN   TestWatchedDependenciesAreUpToDate/github.com/openshift/hypershift/api
    dependencies_test.go:42: Current github.com/openshift/hypershift/api version: v0.0.0-20251024225833-7a63e46b0d15
    dependencies_test.go:48: Current commit hash: 7a63e46b0d15
    dependencies_test.go:54: Latest upstream commit hash: 7a63e46b0d15
    dependencies_test.go:80: ✅ Dependency github.com/openshift/hypershift/api is up-to-date with upstream main branch
=== NAME  TestWatchedDependenciesAreUpToDate
    dependencies_test.go:89: ✅ All 1 watched dependencies are up-to-date
--- PASS: TestWatchedDependenciesAreUpToDate (0.85s)
```

## Fixing Failed Tests

If the test fails due to an outdated dependency:

```bash
# Update specific dependency to latest main branch
go get github.com/openshift/hypershift/api@main

# Or update all watched dependencies
go get github.com/openshift/hypershift/api@main

# Clean up dependencies
go mod tidy

# Update vendor directory
go mod vendor

# Verify the fix
go test ./tests/integration/dependencies -v
```

## Adding New Dependency Monitoring

To monitor additional dependencies:

1. **Add to the watchedDependencies map** in `dependencies_test.go`:
   ```go
   var watchedDependencies = map[string]string{
       "github.com/openshift/hypershift/api": "https://github.com/openshift/hypershift",
       "github.com/your/new-dependency":      "https://github.com/your/repo",
   }
   ```

2. **That's it!** The test will automatically include the new dependency in validation

## Benefits of This Approach

- **Scalable**: Easy to add new dependencies without code duplication
- **Accurate**: Compares against actual upstream commits, not time-based heuristics
- **Informative**: Provides specific commit hashes and update commands
- **Maintainable**: Single configuration point for all watched dependencies