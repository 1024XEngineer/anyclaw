package security

import "time"

// PairingRecord stores trusted-device metadata.
type PairingRecord struct {
	DeviceID   string
	Role       Role
	ApprovedAt time.Time
}
