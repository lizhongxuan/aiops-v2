package selfopt

import "time"

var nowUTC = func() time.Time {
	return time.Now().UTC()
}
