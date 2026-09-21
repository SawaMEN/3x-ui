package swap

import "errors"

var ErrUnsupported = errors.New("swap management is only supported on Linux")

type SwapArea struct {
	Path      string `json:"path"`
	Type      string `json:"type"`
	SizeBytes uint64 `json:"sizeBytes"`
	UsedBytes uint64 `json:"usedBytes"`
	Priority  int    `json:"priority"`
	Active    bool   `json:"active"`
	Managed   bool   `json:"managed"`
}

type ZramArea struct {
	Device           string   `json:"device"`
	ID               int      `json:"id"`
	SizeBytes        uint64   `json:"sizeBytes"`
	UsedBytes        uint64   `json:"usedBytes"`
	CompressedBytes  uint64   `json:"compressedBytes"`
	MemoryUsedBytes  uint64   `json:"memoryUsedBytes"`
	MemoryLimitBytes uint64   `json:"memoryLimitBytes"`
	Algorithm        string   `json:"algorithm"`
	Algorithms       []string `json:"algorithms"`
	Streams          uint64   `json:"streams"`
	Priority         int      `json:"priority"`
	Active           bool     `json:"active"`
	Managed          bool     `json:"managed"`
}

type SwapFileConfig struct {
	Enabled   bool   `json:"enabled"`
	Path      string `json:"path"`
	SizeBytes uint64 `json:"sizeBytes"`
	Priority  int    `json:"priority"`
}

type ZramConfig struct {
	Enabled          bool   `json:"enabled"`
	Device           string `json:"device"`
	SizeBytes        uint64 `json:"sizeBytes"`
	Algorithm        string `json:"algorithm"`
	Streams          uint64 `json:"streams"`
	MemoryLimitBytes uint64 `json:"memoryLimitBytes"`
	Priority         int    `json:"priority"`
}

type Recommendation struct {
	TotalRAMBytes         uint64 `json:"totalRamBytes"`
	CPUCount              int    `json:"cpuCount"`
	RecommendedSwapMiB    int    `json:"recommendedSwapMiB"`
	RecommendedZramMiB    int    `json:"recommendedZramMiB"`
	RecommendedSwappiness int    `json:"recommendedSwappiness"`
	Reason                string `json:"reason"`
}

type ZramPackage struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Installed bool   `json:"installed"`
}

type ZramInstallInfo struct {
	Distribution         string        `json:"distribution"`
	Version              string        `json:"version"`
	PackageManager       string        `json:"packageManager"`
	Package              string        `json:"package"`
	Installed            bool          `json:"installed"`
	Supported            bool          `json:"supported"`
	UsingGenerator       bool          `json:"usingGenerator"`
	InstalledPackages    []ZramPackage `json:"installedPackages"`
	RecommendedPackage   string        `json:"recommendedPackage"`
	RecommendedVersion   string        `json:"recommendedVersion"`
	RecommendedInstalled bool          `json:"recommendedInstalled"`
	ActiveBackend        string        `json:"activeBackend"`
	InstallCommand       string        `json:"installCommand"`
	ReinstallCommand     string        `json:"reinstallCommand"`
	ConfigPath           string        `json:"configPath"`
}

type Config struct {
	SwapFile   SwapFileConfig `json:"swapFile"`
	Zram       ZramConfig     `json:"zram"`
	Swappiness int            `json:"swappiness"`
}

type Status struct {
	TotalBytes        uint64          `json:"totalBytes"`
	UsedBytes         uint64          `json:"usedBytes"`
	Swappiness        int             `json:"swappiness"`
	Areas             []SwapArea      `json:"areas"`
	Zram              []ZramArea      `json:"zram"`
	ZramAlgorithms    []string        `json:"zramAlgorithms"`
	SwapFileConfig    SwapFileConfig  `json:"swapFileConfig"`
	ZramConfig        ZramConfig      `json:"zramConfig"`
	RAMTotalBytes     uint64          `json:"ramTotalBytes"`
	RAMAvailableBytes uint64          `json:"ramAvailableBytes"`
	CPUCount          int             `json:"cpuCount"`
	Recommendation    Recommendation  `json:"recommendation"`
	ZramInstall       ZramInstallInfo `json:"zramInstall"`
}
