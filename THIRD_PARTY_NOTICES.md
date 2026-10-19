# Third-party notices for CI Trust

CI Trust source is MIT licensed; see `LICENSE`. The Go binaries also contain
third-party software under its own terms. Preserve this file and `LICENSES/`
when redistributing the binary archives. Upstream dependency source is unmodified.

| Component | Version / source | Applicable texts included |
| --- | --- | --- |
| Go runtime / standard library | [Go 1.26 series](https://go.dev/LICENSE), exact compiler recorded in `RELEASE.json` | `LICENSES/go-LICENSE` (BSD-3-Clause) |
| JSON Schema validator | [github.com/santhosh-tekuri/jsonschema/v6 v6.0.3](https://github.com/santhosh-tekuri/jsonschema/tree/v6.0.3) | `LICENSES/jsonschema-LICENSE` (Apache-2.0, full terms) |
| YAML parser | [go.yaml.in/yaml/v3 v3.0.4](https://go.yaml.in/yaml/v3) | `LICENSES/yaml-LICENSE` (MIT + Apache-2.0), `LICENSES/yaml-NOTICE`; full Apache terms above |
| Terminal support | [golang.org/x/term v0.47.0](https://pkg.go.dev/golang.org/x/term@v0.47.0) | `LICENSES/x-term-LICENSE` (BSD-3-Clause), `LICENSES/x-term-PATENTS` |
| System support | [golang.org/x/sys v0.49.0](https://pkg.go.dev/golang.org/x/sys@v0.49.0) | `LICENSES/x-sys-LICENSE` (BSD-3-Clause), `LICENSES/x-sys-PATENTS` |
| Text support | [golang.org/x/text v0.14.0](https://pkg.go.dev/golang.org/x/text@v0.14.0) | `LICENSES/x-text-LICENSE` (BSD-3-Clause), `LICENSES/x-text-PATENTS` |

The module versions come from the pinned `go.mod`/`go.sum` and linked Go package
inventory. License/notice/patent texts are copied unchanged from those modules;
the Go license is the official BSD text (verified unchanged in Go 1.26.4/1.26.9).
Each native archive records its actual compiler version in `RELEASE.json`.
Original copyright notices and warranty disclaimers remain included. Apache-2.0 applies to
upstream components, not a relicensing of CI Trust's own source.

The binary archives do not bundle Node.js, npm, ESLint, a container image or the
experimental lint candidate. Those separately prepared components retain their
own terms; this notice is not a distribution-license review for those artifacts.
