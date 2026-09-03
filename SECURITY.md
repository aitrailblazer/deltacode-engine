# Security

## Supported versions

Only the latest tagged release receives security fixes.

## Reporting

Do not open a public issue for a suspected vulnerability. Use GitHub's private
vulnerability reporting for this repository.

## Trust boundary

DeltaCode is a read-only context tool. It does not grant edit or command
authority. File operations are constrained to a caller-declared canonical
repository root, and symlinks and root escapes are rejected.
