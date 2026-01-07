# Integration Tests

This directory contains integration tests for the hypershift-oadp-plugin.

## Structure

```
tests/integration/
├── dependencies/          # Dependency validation tests
│   └── dependencies_test.go
├── backup/               # Future: Backup operation tests
├── restore/              # Future: Restore operation tests
└── networking/           # Future: Network-related tests
```

## Test Categories

### Dependencies (`./dependencies/`)

Tests that validate project dependencies are up-to-date and compatible.

- **Purpose**: Prevent scheme compatibility issues and other dependency-related problems
- **Frequency**: Should run on every CI build
- **Focus**: Critical dependencies that affect plugin functionality

### Running Tests

```bash
# Run all integration tests
go test ./tests/integration/... -v

# Run specific category
go test ./tests/integration/dependencies -v

# Run from project root
cd /path/to/hypershift-oadp-plugin
go test ./tests/integration/... -v
```

## Adding New Test Categories

When adding new integration test categories:

1. Create a new subdirectory under `tests/integration/`
2. Use meaningful package names (e.g., `package backup`, `package restore`)
3. Add tests that focus on integration scenarios rather than unit testing
4. Update this README with the new category

## CI Integration

These tests are designed to catch issues early in the development cycle:

- **Dependencies**: Ensures critical dependencies don't become stale
- **Future categories**: Will test actual plugin functionality in realistic scenarios