package importreceipt

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const (
	BundleHeader = "X-PaperSage-Import-Bundle-Id"
	LocalHeader  = "X-PaperSage-Import-Local-Id"
)

var (
	ErrInvalidHeaders = errors.New("both import headers must be valid UUIDs")
	ErrConflict       = errors.New("import key was already used for different content")
)

type Key struct {
	BundleID uuid.UUID
	LocalID  uuid.UUID
}

// Parse accepts either both absent (ordinary request) or two valid UUIDs.
func Parse(headers http.Header) (Key, bool, error) {
	bundleRaw, localRaw := headers.Get(BundleHeader), headers.Get(LocalHeader)
	if bundleRaw == "" && localRaw == "" {
		return Key{}, false, nil
	}
	bundleID, bundleErr := uuid.Parse(bundleRaw)
	localID, localErr := uuid.Parse(localRaw)
	if bundleErr != nil || localErr != nil || bundleID == uuid.Nil || localID == uuid.Nil {
		return Key{}, true, ErrInvalidHeaders
	}
	return Key{BundleID: bundleID, LocalID: localID}, true, nil
}

func Fingerprint(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// Claim inserts a receipt in the caller's transaction. Resource rows must be
// created in that same transaction. A conflicting concurrent insert waits for
// the winner, then returns its durable resource ID for replay.
func Claim(tx *gorm.DB, ownerID store.UUID, key Key, resourceType, fingerprint string, proposedResourceID store.UUID) (resourceID store.UUID, replay bool, err error) {
	candidate := store.ImportReceipt{
		ID: store.MustNewUUID(), OwnerID: ownerID,
		BundleID: store.UUID{UUID: key.BundleID}, ResourceType: resourceType,
		LocalID: store.UUID{UUID: key.LocalID}, Fingerprint: fingerprint,
		ResourceID: proposedResourceID,
	}
	result := tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "owner_id"}, {Name: "bundle_id"}, {Name: "resource_type"}, {Name: "local_id"}},
		DoNothing: true,
	}).Create(&candidate)
	if result.Error != nil {
		return store.UUID{}, false, result.Error
	}
	if result.RowsAffected == 1 {
		return proposedResourceID, false, nil
	}

	var existing store.ImportReceipt
	err = tx.Where("owner_id = ? AND bundle_id = ? AND resource_type = ? AND local_id = ?",
		ownerID, candidate.BundleID, resourceType, candidate.LocalID).First(&existing).Error
	if err != nil {
		return store.UUID{}, false, err
	}
	if existing.Fingerprint != fingerprint {
		return store.UUID{}, false, ErrConflict
	}
	return existing.ResourceID, true, nil
}
