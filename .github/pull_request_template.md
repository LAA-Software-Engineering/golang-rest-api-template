## Summary

<!-- What does this PR change and why? Link any design notes or prior discussion. -->

## Type of change

<!-- Mark the relevant option with an “x” (e.g. `- [x] Bug fix`). -->

- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change (describe impact below)
- [ ] Documentation only
- [ ] Build / CI / tooling
- [ ] Dependency update

## How to test

<!-- Steps you ran or reviewers should run. -->

```bash
go test ./... -race
```

<!-- If you changed HTTP behavior, Docker, or integration tests: -->

```bash
# optional: local stack
# make up

# optional: Python E2E (see README; requires BASE_URL and API_KEY)
# pytest -v tests/e2e.py
```

## Checklist

- [ ] `go test ./... -race` passes locally
- [ ] If you changed **routes, handlers, or models**: Swagger was regenerated (`swag init` / project `Makefile` `setup` target) and `docs/` is updated if required
- [ ] If you added or renamed **environment variables**: README and/or `.env` examples are updated
- [ ] If you changed **Docker or compose**: `docker compose build` (or `make build-docker`) still succeeds
- [ ] No new secrets, credentials, or production keys committed

## Related issues

<!-- e.g. Closes #123 -->

-
