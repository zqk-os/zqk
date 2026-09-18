package qa

import (
	"crypto/rand"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	qasuccessenum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/qa_success"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
)

// qaSuccessTitlePrefix is the human-readable name for a minted token. base_object
// requires title; the signed payload remains item_id+"success" and does not cover title.
const qaSuccessTitlePrefix = "QA success for "

// QASuccessTitle returns the required title for a qa_success token covering itemID.
func QASuccessTitle(itemID string) string {
	return qaSuccessTitlePrefix + itemID
}

func newQASuccessID() string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Sprintf("QAS-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("QAS-%d-%x", time.Now().UnixNano(), suffix)
}

func buildQASuccessObject(itemID, signature, publicKey string) (map[string]any, error) {
	b := bldr_instance_v1.NewQaSuccessInstanceBuilder(objects.DefaultSchemaVersion)
	b.ID(newQASuccessID()).
		Title(QASuccessTitle(itemID)).
		ItemId(itemID).
		Status(qasuccessenum.StatusSuccess).
		Signature(signature).
		PublicKey(publicKey)
	return b.Build()
}
