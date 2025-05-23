# Caching

- Enabled in production by default
- Disabled in dev unless overridden
- Cached files are saved to `outputDir` (default: `out/`)

## Cache Headers

- `X-Barry-Cache: MISS` → first request
- `X-Barry-Cache: HIT`  → served from cache
