package httpapi

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestOpenAPIPathsMatchRegisteredRoutes(t *testing.T) {
	serverSource, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	contract, err := os.ReadFile("../../../../packages/api-contracts/platform-api.yaml")
	if err != nil {
		t.Fatal(err)
	}

	routePattern := regexp.MustCompile(`HandleFunc\("(GET|POST|PUT|PATCH|DELETE) ([^"]+)"`)
	registered := map[string]bool{}
	for _, match := range routePattern.FindAllStringSubmatch(string(serverSource), -1) {
		registered[match[1]+" "+match[2]] = true
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
