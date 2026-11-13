package sidecartraffic

// WithActivity retains zero-byte activity by pinned identity, including first
// connection events used by delayed-start expiration maintenance.
func WithActivity(samples []Sample, online []string) []Sample {
	seen := make(map[string]int, len(samples))
	for i, sample := range samples {
		seen[sample.Email] = i
	}
	for _, email := range online {
		if i, ok := seen[email]; ok {
			samples[i].Active = true
		} else {
			seen[email] = len(samples)
			samples = append(samples, Sample{Email: email, Active: true})
		}
	}
	return samples
}
