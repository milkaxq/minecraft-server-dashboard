package server

// ValidSoftware is the allowlist shared by orders and operator configuration.
func ValidSoftware(kind string) bool {
	switch kind {
	case "VANILLA", "PAPER", "SPIGOT", "PURPUR", "FABRIC", "FORGE", "NEOFORGE":
		return true
	}
	return false
}

func SupportsPlugins(kind string) bool {
	return kind == "PAPER" || kind == "SPIGOT" || kind == "PURPUR"
}

func SupportsMods(kind string) bool {
	return kind == "FABRIC" || kind == "FORGE" || kind == "NEOFORGE"
}

// NewSoftware keeps the customer offering small without removing support for
// existing servers or their provisioning retries.
func NewSoftware(kind string) bool {
	return kind == "FORGE" || kind == "PAPER" || kind == "FABRIC"
}
