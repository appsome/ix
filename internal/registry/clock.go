package registry

import "time"

// timestamp returns a UTC migration-style timestamp (YYYYMMDDHHMMSS) used for
// once-rendered migration filenames. It is a package var so tests can stub it.
var timestamp = func() string {
	return time.Now().UTC().Format("20060102150405")
}
