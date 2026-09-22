// BLI-STARTER-COMMUNITY-016 / PRI-STARTER-COMMUNITY-048
package quality

import "testing"

func TestEvidenceListMatricesMissingRegistry(t *testing.T) {
	if _, err := ListMatricesFromRegistry(t.TempDir(), "missing.yaml"); err == nil {
		t.Fatal("explicit missing registry must fail closed")
	}
}
