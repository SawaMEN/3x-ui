package singbox

func NewProcess(configPath string) *Process {
	return &Process{config: resolvePath(configPath), version: "Unknown"}
}

// NewTestProcess never discovers or stops the managed production process.
func NewTestProcess(configPath string) *Process {
	return &Process{config: resolvePath(configPath), version: "Unknown", isolated: true}
}
