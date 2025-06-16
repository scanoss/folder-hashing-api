package entities

// ScanResponse represents the domain model for folder hash scanning response
type ScanResponse struct {
	// List of folders containing unique components
	Results []*ScanResult
}

// ScanResult represents a scan result for a specific path
type ScanResult struct {
	// Folder path (can be actual or obfuscated)
	PathID string
	// List of matching component groups
	ComponentGroups []*ComponentGroup
}

// StatusResponse represents status information
type StatusResponse struct {
	Code    int32
	Message string
}
