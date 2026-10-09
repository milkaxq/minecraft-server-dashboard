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
