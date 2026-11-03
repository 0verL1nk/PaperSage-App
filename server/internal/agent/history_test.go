package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/config"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

func TestCompleteConversationHistory(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(t.TempDir()+"/history.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	require.NoError(t, db.AutoMigrate(&store.User{}, &store.AgentConversation{}, &store.AgentMessage{}, &store.AgentMessagePart{}))

	uid, other := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&store.User{BaseModel: store.BaseModel{ID: uid}, Username: "history-user", PasswordHash: "x"}).Error)
	conv, foreign := store.MustNewUUID(), store.MustNewUUID()
	pid := store.MustNewUUID()
	require.NoError(t, db.Create(&[]store.AgentConversation{
		{ID: conv, ProjectID: pid, UserID: uid},
		{ID: foreign, ProjectID: pid, UserID: other},
	}).Error)

	const count = 1100
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	messageID := func(i int) store.UUID {
		return store.UUID{UUID: uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012x", i+1))}
	}
	msgs := make([]store.AgentMessage, 0, count)
	parts := make([]store.AgentMessagePart, 0, count*2)
	for i := count - 1; i >= 0; i-- {
		created := base
		if i == 0 {
			created = base.Add(time.Second)
		}
		id := messageID(i)
		msgs = append(msgs, store.AgentMessage{ID: id, ConversationID: conv, Role: "user", Status: "completed", CreatedAt: created})
		first, second := fmt.Sprintf("message-%d/", i), "end"
		parts = append(parts,
			store.AgentMessagePart{ID: store.MustNewUUID(), MessageID: id, Ordinal: 1, Type: PartText, Status: "completed", TextContent: &second},
			store.AgentMessagePart{ID: store.MustNewUUID(), MessageID: id, Ordinal: 0, Type: PartText, Status: "completed", TextContent: &first})
		if i == 1 {
			parts = append(parts, store.AgentMessagePart{ID: store.MustNewUUID(), MessageID: id, Ordinal: 2, Type: "tool", Status: "completed", ContentJSON: []byte(`{"result":"ok"}`)})
		}
	}
	require.NoError(t, db.CreateInBatches(msgs, 50).Error)
	require.NoError(t, db.CreateInBatches(parts, 50).Error)
	emptyID := store.MustNewUUID()
	require.NoError(t, db.Create(&store.AgentMessage{ID: emptyID, ConversationID: conv, Role: "assistant", Status: "completed", CreatedAt: base.Add(-time.Second)}).Error)
	deletedID, foreignID := store.MustNewUUID(), store.MustNewUUID()
	require.NoError(t, db.Create(&[]store.AgentMessage{
		{ID: deletedID, ConversationID: conv, Role: "user", Status: "completed"},
		{ID: foreignID, ConversationID: foreign, Role: "user", Status: "completed"},
	}).Error)
	require.NoError(t, db.Delete(&store.AgentMessage{}, "id = ?", deletedID).Error)
	secret := "must-not-be-read"
	require.NoError(t, db.Create(&[]store.AgentMessagePart{
		{ID: store.MustNewUUID(), MessageID: deletedID, Ordinal: 0, Type: PartText, Status: "completed", TextContent: &secret},
		{ID: store.MustNewUUID(), MessageID: foreignID, Ordinal: 0, Type: PartText, Status: "completed", TextContent: &secret},
	}).Error)

	// Observe the real joined reads to ensure scope and parameter count do not depend on history length.
	var maxParams, historyQueries int
	require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:history-read", func(tx *gorm.DB) {
		if tx.Statement.Table != "agent_messages" {
			return
		}
		historyQueries++
		if len(tx.Statement.Vars) > maxParams {
			maxParams = len(tx.Statement.Vars)
		}
		got, ok := tx.Statement.Dest.(*[]historyRow)
		require.True(t, ok, "history read must scan joined rows")
		for _, row := range *got {
			if row.ID == deletedID || row.ID == foreignID {
				t.Errorf("history query read a deleted or foreign message: %s", row.ID.String())
			}
		}
	}))

	cfg := &config.Settings{SecretKey: "test"}
	svc := NewService(db, nil, nil, cfg, nil)
	api := httpapi.New(nil)
	Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), svc)
	token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
	require.NoError(t, err)
	get := func(id store.UUID) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/conversations/"+id.String()+"/messages", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		api.ServeHTTP(rec, req)
		return rec
	}
	rec := get(conv)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body struct {
		Data []msgDTO `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
	require.Equal(t, count+1, len(body.Data), "all messages, including those after the 200th, must be visible")
	require.Equal(t, emptyID.String(), body.Data[0].ID)
	require.Empty(t, body.Data[0].Parts, "messages without parts must be preserved")
	for pos, got := range body.Data[1:] {
		i := (pos + 1) % count
		require.Equal(t, messageID(i).String(), got.ID, "created_at then id ordering at %d", pos)
		if i == 1 {
			require.Len(t, got.Parts, 3)
			require.Equal(t, "tool", got.Parts[2].Type)
			require.JSONEq(t, `{"result":"ok"}`, string(got.Parts[2].Content))
		} else {
			require.Len(t, got.Parts, 2)
		}
		require.Equal(t, 0, got.Parts[0].Ordinal)
		require.Equal(t, 1, got.Parts[1].Ordinal)
		require.Equal(t, fmt.Sprintf("message-%d/", i), *got.Parts[0].Text)
		require.Equal(t, "end", *got.Parts[1].Text)
	}
	require.Equal(t, http.StatusNotFound, get(foreign).Code)

	history, err := svc.loadHistory(context.Background(), conv)
	require.NoError(t, err)
	require.Len(t, history, count+2)
	require.Empty(t, history[1].Content)
	for pos, msg := range history[2:] {
		i := (pos + 1) % count
		require.Equal(t, fmt.Sprintf("message-%d/end", i), msg.Content, "history at %d", pos)
	}
	require.Equal(t, 2, historyQueries)
	require.Equal(t, 1, maxParams, "history queries must not bind every message ID")

	require.NoError(t, db.Callback().Query().Before("gorm:query").Register("test:authorization-error", func(tx *gorm.DB) {
		if tx.Statement.Table == "agent_conversations" {
			_ = tx.AddError(errors.New("authorization database unavailable"))
		}
	}))
	require.Equal(t, http.StatusInternalServerError, get(conv).Code, "authorization query failures must not be masked as 404")
}

func TestHistoryConcurrentConversationDeletion(t *testing.T) {
	for _, tc := range []struct{ path, deletion string }{{"api", "soft"}, {"model", "soft"}, {"api", "hard"}, {"model", "hard"}} {
		t.Run(tc.path+"/"+tc.deletion, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(t.TempDir()+"/deletion.db"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&store.User{}, &store.AgentConversation{}, &store.AgentMessage{}, &store.AgentMessagePart{}))
			uid, conv, msgID := store.MustNewUUID(), store.MustNewUUID(), store.MustNewUUID()
			require.NoError(t, db.Create(&store.User{BaseModel: store.BaseModel{ID: uid}, Username: "race-user", PasswordHash: "x"}).Error)
			require.NoError(t, db.Create(&store.AgentConversation{ID: conv, UserID: uid, ProjectID: store.MustNewUUID()}).Error)
			require.NoError(t, db.Create(&store.AgentMessage{ID: msgID, ConversationID: conv, Role: "user", Status: "completed"}).Error)
			text := "original message"
			require.NoError(t, db.Create(&store.AgentMessagePart{ID: store.MustNewUUID(), MessageID: msgID, Ordinal: 0, Type: PartText, Status: "completed", TextContent: &text}).Error)

			// Commit the same soft deletions as deleteConversation after the message query,
			// before the caller has a chance to issue the parts query.
			var injected bool
			var deleteErr error
			require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:delete-after-message-read", func(tx *gorm.DB) {
				if injected || tx.Statement.Table != "agent_messages" {
					return
				}
				injected = true
				deleteErr = db.Transaction(func(tx *gorm.DB) error {
					if tc.deletion == "hard" {
						if err := tx.Where("message_id = ?", msgID).Delete(&store.AgentMessagePart{}).Error; err != nil {
							return err
						}
						tx = tx.Unscoped()
					}
					if err := tx.Where("conversation_id = ?", conv).Delete(&store.AgentMessage{}).Error; err != nil {
						return err
					}
					return tx.Delete(&store.AgentConversation{}, "id = ?", conv).Error
				})
			}))
			cfg := &config.Settings{SecretKey: "test"}
			svc := NewService(db, nil, nil, cfg, nil)
			if tc.path == "api" {
				api := httpapi.New(nil)
				Mount(api.Group(httpapi.APIVersion), auth.NewService(db, cfg, nil).AuthRequired(), svc)
				token, err := auth.CreateAccessToken(cfg.SecretKey, uid.String(), 30)
				require.NoError(t, err)
				req := httptest.NewRequest(http.MethodGet, "/api/v1/agent/conversations/"+conv.String()+"/messages", nil)
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				api.ServeHTTP(rec, req)
				require.True(t, injected)
				require.NoError(t, deleteErr)
				if rec.Code == http.StatusOK {
					var body struct {
						Data []msgDTO `json:"data"`
					}
					require.NoError(t, json.NewDecoder(rec.Body).Decode(&body))
					for _, msg := range body.Data {
						require.Len(t, msg.Parts, 1, "must not return a message with its text lost during deletion")
						require.Equal(t, text, *msg.Parts[0].Text)
					}
				} else {
					require.Equal(t, http.StatusNotFound, rec.Code)
				}
				return
			}
			history, err := svc.loadHistory(context.Background(), conv)
			require.True(t, injected)
			require.NoError(t, deleteErr)
			if err == nil {
				for _, msg := range history[1:] {
					require.Equal(t, text, msg.Content, "must not send a message with its text lost during deletion")
				}
			}
		})
	}
}
