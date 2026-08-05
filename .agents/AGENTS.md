# Workspace Rules

## Test Coverage Rule
- All modules and packages must have at least 80% unit test coverage.
- All unit tests must pass cleanly before any code is pushed (`git push`).

## Documentation Maintenance Rule (Pre-Push Requirement)
- Before executing any code push (`git push`), you MUST check whether any architectural changes, new endpoints, package updates, bug fixes, or schema modifications were introduced.
- You MUST update `CLAUDE.md` to ensure all system details (providers, endpoints, hypervisor features, scheduler rules, admin capabilities, configuration flags) remain 100% complete and up-to-date for all AI agents.

