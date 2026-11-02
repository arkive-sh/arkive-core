# Third-party notices

Arkive Core includes third-party software:

- Gin — MIT
- PGX — MIT
- AWS SDK Go v2 — Apache-2.0
- golang.org/x/crypto — BSD-3-Clause
- godotenv — MIT
- Gomponents — MIT
- Gomponents Lucide — MIT
- Plyr — MIT
- libheif 1.23.7 — LGPL-3.0-or-later
- libde265 1.1.3 — LGPL-3.0-or-later

See respective upstream repositories for license details.

Plyr is vendored under `core/web/static/vendor/plyr/` and is MIT licensed.

The HEIC/HEIF browser fallback is built from pinned upstream libheif and libde265 source revisions. Their license texts are shipped with the generated WebAssembly assets under `web/static/wasm/heic/`.

Arkive Crypto under `core/web/static/vendor/arkive-crypto/` is maintained by Arkive.
