package brewrestart

import "errors"

// ErrPreparedProcessPresent is returned only after positive kernel executable
// attribution to the canonical prepared generation. It permits waiting for exit,
// never signaling that process or relaxing the final absence requirement.
var ErrPreparedProcessPresent = errors.New("source generation process still present")
