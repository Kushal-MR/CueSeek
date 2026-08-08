module github.com/Kushal-MR/CueSeek/agent

go 1.25.0

// The `go` directive above is raised by dependencies, not chosen: modernc.org/sqlite
// v1.56.0 requires 1.25. Anything building this agent needs at least that.
//
// Still expected, per the architecture:
//   github.com/godbus/dbus/v5          systemd + logind          (ADR-0002)
//   github.com/coreos/go-systemd/v22   unit state helpers        (ADR-0002)
//   github.com/shirou/gopsutil/v4      host metrics              (ADR-0008)
//
// Already present:
//   modernc.org/sqlite                 device registry + audit   (ADR-0006)
//                                      cgo-free, so the agent stays a single static
//                                      binary with no libsqlite3 on the target host
//   gopkg.in/yaml.v3                   configuration

// Pinned so that `go generate` produces byte-identical output here and in CI.
// An unpinned generator turns the drift gate into a source of spurious failures.
tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen

require (
	github.com/getkin/kin-openapi v0.127.0
	github.com/oapi-codegen/runtime v1.6.0
	gopkg.in/yaml.v3 v3.0.1
	modernc.org/sqlite v1.56.0
)

require (
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/dprotaso/go-yit v0.0.0-20220510233725-9ba8df137936 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/invopop/yaml v0.3.1 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/oapi-codegen/oapi-codegen/v2 v2.4.1 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/speakeasy-api/openapi-overlay v0.9.0 // indirect
	github.com/vmware-labs/yaml-jsonpath v0.3.2 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	modernc.org/libc v1.74.4 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.11.0 // indirect
)
