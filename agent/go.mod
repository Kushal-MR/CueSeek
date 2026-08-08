module github.com/Kushal-MR/CueSeek/agent

go 1.24

// No dependencies yet. Expected, per the architecture:
//   github.com/godbus/dbus/v5          systemd + logind          (ADR-0002)
//   github.com/coreos/go-systemd/v22   unit state helpers        (ADR-0002)
//   github.com/shirou/gopsutil/v4      host metrics              (ADR-0008)
//   modernc.org/sqlite                 device registry + audit   (ADR-0006)
//                                      (cgo-free, keeps the static-binary property)
