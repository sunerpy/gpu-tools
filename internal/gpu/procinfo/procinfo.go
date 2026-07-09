package procinfo

import "os/user"

var (
	procRoot     = "/proc"
	lookupUserID = user.LookupId
)
