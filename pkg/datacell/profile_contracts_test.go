package datacell

import "testing"

func TestContractForProfile_KnownProfiles(t *testing.T) {
	t.Parallel()
	tests := []StorageProfile{
		ProfileCASEntity,
		ProfileLightFile,
		ProfileStream,
	}
	for _, profile := range tests {
		profile := profile
		t.Run(profile.String(), func(t *testing.T) {
			t.Parallel()
			contract, ok := ContractForProfile(profile)
			if !ok {
				t.Fatalf("expected contract for %q", profile)
			}
			if contract.Profile != profile {
				t.Fatalf("profile mismatch: got %q want %q", contract.Profile, profile)
			}
			if contract.WriteMode == "" || contract.ReadMode == "" || contract.IntegrityMode == "" || contract.MigrationMode == "" {
				t.Fatalf("contract fields must be non-empty: %#v", contract)
			}
			if contract.MembraneTransport == "" || contract.MembranePolicy == "" {
				t.Fatalf("membrane contract fields must be non-empty (CRIT-DATACELL-002): %#v", contract)
			}
		})
	}
}

func TestContractForProfile_UnknownProfile(t *testing.T) {
	t.Parallel()
	if _, ok := ContractForProfile(StorageProfile("unknown")); ok {
		t.Fatal("expected unknown profile to have no contract")
	}
}

// [REDACTED-ID]: profileContracts must stay in lockstep with KnownStorageProfiles
// (table-driven gate — no orphan map entries or missing contracts).
func TestProfileContractsMapMatchesKnownStorageProfiles(t *testing.T) {
	t.Parallel()
	if len(KnownStorageProfiles) != len(profileContracts) {
		t.Fatalf("KnownStorageProfiles len %d != profileContracts len %d — add contract or list entry",
			len(KnownStorageProfiles), len(profileContracts))
	}
	seen := make(map[StorageProfile]bool, len(KnownStorageProfiles))
	for _, p := range KnownStorageProfiles {
		if seen[p] {
			t.Fatalf("duplicate in KnownStorageProfiles: %q", p)
		}
		seen[p] = true
		c, ok := ContractForProfile(p)
		if !ok {
			t.Fatalf("KnownStorageProfiles entry %q has no contract row", p)
		}
		if c.Profile != p {
			t.Fatalf("contract profile mismatch for %q: %#v", p, c)
		}
	}
	for p := range profileContracts {
		if !seen[p] {
			t.Fatalf("profileContracts has key %q not listed in KnownStorageProfiles", p)
		}
	}
}
