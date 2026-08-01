package api_builders

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// HTTPClient is an interface that matches *http.Client and specbuilder.APIClient.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// APIBuilder defines a fluent interface for constructing and executing HTTP API requests.
type APIBuilder interface {
	SetURL(url string) APIBuilder
	SetMethod(method string) APIBuilder
	AddHeader(key, value string) APIBuilder
	SetBody(body []byte) APIBuilder
	SetTimeout(timeout time.Duration) APIBuilder
	SetClient(client HTTPClient) APIBuilder
	SetRoundTripper(rt http.RoundTripper) APIBuilder
	SetLogger(logger *logging.EventLogger) APIBuilder
	Build(ctx context.Context) (*http.Response, error)
}

const emptyValue = ""

// APISpecGenBuilder is an interface for builders that generate api_spec YAML
type APISpecGenBuilder interface {
	Build() ([]byte, error)
	GetVersion() string
	GetFileName() string
}

// VersionedAPISpecGenBuilderRegistry manages versioned api_spec builders
type VersionedAPISpecGenBuilderRegistry struct {
	builders map[string]map[string]APISpecGenBuilder // fileName -> version -> builder
}

// NewVersionedAPISpecGenBuilderRegistry creates a new registry
func NewVersionedAPISpecGenBuilderRegistry() *VersionedAPISpecGenBuilderRegistry {
	return &VersionedAPISpecGenBuilderRegistry{
		builders: make(map[string]map[string]APISpecGenBuilder),
	}
}

// Register registers a builder for a specific file name and version
func (r *VersionedAPISpecGenBuilderRegistry) Register(builder APISpecGenBuilder) {
	fileName := builder.GetFileName()
	version := builder.GetVersion()

	if r.builders[fileName] == nil {
		r.builders[fileName] = make(map[string]APISpecGenBuilder)
	}
	r.builders[fileName][version] = builder
}

// GetBuilder returns the builder for a specific file name and version
func (r *VersionedAPISpecGenBuilderRegistry) GetBuilder(fileName, version string) (APISpecGenBuilder, error) {
	if r.builders[fileName] == nil {
		return nil, errfmt.Errorf("no builders found for api_spec: %s", fileName)
	}
	builder, ok := r.builders[fileName][version]
	if !ok {
		return nil, errfmt.Errorf("no builder found for api_spec %s at version %s", fileName, version)
	}
	return builder, nil
}

// GetLatestVersion returns the latest version for a file name
func (r *VersionedAPISpecGenBuilderRegistry) GetLatestVersion(fileName string) (string, error) {
	if len(r.builders[fileName]) == 0 {
		return "", errfmt.Errorf("no builders found for api_spec: %s", fileName)
	}

	// Find highest semantic version
	latestVersion := ""
	var maxMajor, maxMinor, maxPatch = -1, -1, -1

	for version := range r.builders[fileName] {
		// Parse semantic version (format "v1_0_0" -> major=1, minor=0, patch=0)
		var major, minor, patch int
		if _, err := fmt.Sscanf(version, "v%d_%d_%d", &major, &minor, &patch); err == nil {
			// Compare semantic versions (major.minor.patch)
			if major > maxMajor ||
				(major == maxMajor && minor > maxMinor) ||
				(major == maxMajor && minor == maxMinor && patch > maxPatch) {
				maxMajor = major
				maxMinor = minor
				maxPatch = patch
				latestVersion = version
			}
		}
	}

	if latestVersion == emptyValue {
		// Fallback: return any version if parsing failed
		for version := range r.builders[fileName] {
			return version, nil
		}
	}

	return latestVersion, nil
}

// GetAllFileNames returns all api_spec file names with builders
func (r *VersionedAPISpecGenBuilderRegistry) GetAllFileNames() []string {
	fileNames := make([]string, 0, len(r.builders))
	for fileName := range r.builders {
		fileNames = append(fileNames, fileName)
	}
	return fileNames
}

// GetVersions returns all versions for a file name
func (r *VersionedAPISpecGenBuilderRegistry) GetVersions(fileName string) []string {
	if r.builders[fileName] == nil {
		return nil
	}
	versions := make([]string, 0, len(r.builders[fileName]))
	for version := range r.builders[fileName] {
		versions = append(versions, version)
	}
	return versions
}
