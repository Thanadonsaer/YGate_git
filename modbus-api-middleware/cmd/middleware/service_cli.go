package main

type serviceConfig struct {
	Action               string
	DatabasePath         string
	Listen               string
	LicenseFile          string
	CleanupRetentionDays int
}
