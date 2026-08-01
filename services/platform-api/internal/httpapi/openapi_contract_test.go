package httpapi

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestOpenAPIPathsMatchRegisteredRoutes(t *testing.T) {
	// packages/api-contracts/platform-api.yaml documents the whole platform's
	// API surface, served behind one gateway -- but login/logout/password
	// management/session listing and the users/roles/permissions/api-keys/
	// profile admin CRUD now live in auth-service's own server.go, not
	// platform-api's (see docs/superpowers/plans/2026-08-01-backend-microservices-phase1-auth-service.md).
	// Union both services' registered routes before comparing against the
	// contract so this test still catches drift between code and docs.
	registered := map[string]bool{}
	routePattern := regexp.MustCompile(`HandleFunc\("(GET|POST|PUT|PATCH|DELETE) ([^"]+)"`)
	for _, path := range []string{"server.go", "../../../auth-service/internal/httpapi/server.go"} {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range routePattern.FindAllStringSubmatch(string(source), -1) {
			registered[match[1]+" "+match[2]] = true
		}
	}
	contract, err := os.ReadFile("../../../../packages/api-contracts/platform-api.yaml")
	if err != nil {
		t.Fatal(err)
	}

	documented := map[string]bool{}
	currentPath := ""
	methodPattern := regexp.MustCompile(`^    (get|post|put|patch|delete):`)
	for _, line := range strings.Split(string(contract), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "  /") && strings.HasSuffix(trimmed, ":") {
			currentPath = strings.TrimSuffix(trimmed, ":")
			continue
		}
		if !strings.HasPrefix(line, " ") {
			currentPath = ""
		}
		if currentPath != "" {
			if match := methodPattern.FindStringSubmatch(line); match != nil {
				documented[strings.ToUpper(match[1])+" "+currentPath] = true
			}
		}
	}

	if missing := routeDifference(registered, documented); len(missing) > 0 {
		t.Fatalf("registered routes missing from OpenAPI: %s", strings.Join(missing, ", "))
	}
	if missing := routeDifference(documented, registered); len(missing) > 0 {
		t.Fatalf("OpenAPI operations missing from server: %s", strings.Join(missing, ", "))
	}
}

func TestOpenAPIIngestionContractMatchesResponseAndCompatibilityFields(t *testing.T) {
	contract, err := os.ReadFile("../../../../packages/api-contracts/platform-api.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(contract)
	for _, required := range []string{
		"acceptedCount:", "duplicateCount:", "rejectedCount:",
		"onboardedPlantCount:", "onboardedDeviceCount:",
		"X-Correlation-ID:", "Idempotency-Key", "Content-Encoding",
		"inverterReading:", "holdingRegisterReading:",
		`enum: ["2.0"]`,
	} {
		if !strings.Contains(text, required) {
			t.Errorf("OpenAPI ingestion contract missing %q", required)
		}
	}
	for _, stale := range []string{"acceptedRecords:", "duplicateRecords:", "rejectedRecords:", "onboardedPlants:", "onboardedDevices:"} {
		if strings.Contains(text, stale) {
			t.Errorf("OpenAPI ingestion contract contains stale response field %q", stale)
		}
	}
}

func routeDifference(left, right map[string]bool) []string {
	difference := []string{}
	for route := range left {
		if !right[route] {
			difference = append(difference, route)
		}
	}
	sort.Strings(difference)
	return difference
}
