package supply

// Verifier provides supply-chain verification and release integrity checks.
type Verifier struct{}

// NewVerifier creates a new supply chain verifier.
func NewVerifier() *Verifier {
	return &Verifier{}
}
