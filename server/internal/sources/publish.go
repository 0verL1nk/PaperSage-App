// Package sources 资料对象与不可变来源版本。
// 版本发布必须与业务写入同一事务：对象已就绪后再落版本行并回填引用。
package sources

import (
	"errors"

	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

// PublishDocument 为已入库文档创建来源与首个版本，并回填 documents.source_id。
// 幂等：已有 source_id 的文档直接返回已有版本。
func PublishDocument(tx *gorm.DB, projectID, documentID, createdBy store.UUID,
	s3Key, contentSHA256 string, size int64) (store.UUID, error) {
	var doc store.Document
	if err := tx.Select("id", "source_id").Where("id = ? AND project_id = ?", documentID, projectID).
		First(&doc).Error; err != nil {
		return store.UUID{}, err
	}
	if doc.SourceID != nil {
		var existing store.SourceVersion
		if err := tx.Where("source_id = ?", *doc.SourceID).Order("revision").First(&existing).Error; err != nil {
			return store.UUID{}, err
		}
		return existing.ID, nil
	}
	src := store.ProjectSource{ProjectID: projectID, Kind: "document", CurrentRevision: 1}
	if err := tx.Create(&src).Error; err != nil {
		return store.UUID{}, err
	}
	ver := store.SourceVersion{
		SourceID: src.ID, Revision: 1, S3Key: s3Key,
		ContentSHA256: contentSHA256, SizeBytes: size, CreatedBy: &createdBy,
	}
	if err := tx.Create(&ver).Error; err != nil {
		return store.UUID{}, err
	}
	res := tx.Model(&store.Document{}).Where("id = ? AND source_id IS NULL", documentID).
		Update("source_id", src.ID)
	if res.Error != nil {
		return store.UUID{}, res.Error
	}
	if res.RowsAffected != 1 {
		return store.UUID{}, errors.New("document source link lost a concurrent race")
	}
	return ver.ID, nil
}
