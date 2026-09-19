# Aixgo Sync

Multi-tenant coordination plane for the Aixgo ecosystem: sessions, messages, jobs, desired-state, and remote control — durable JSON on S3-compatible storage with compare-and-swap.

**PRD:** [docs/PRD.md](docs/PRD.md) · **Issue slices:** [docs/ISSUES.md](docs/ISSUES.md) · **Ecosystem:** [aixgo-dev/product](https://github.com/aixgo-dev/product)

## Status

Early scaffold (M0). Not production-ready.

## Develop

```bash
make check
go run ./cmd/aixgo-sync version
```

## License

Apache-2.0
