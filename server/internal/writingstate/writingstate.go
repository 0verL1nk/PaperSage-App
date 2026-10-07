// Package writingstate stores the editable preparation and phase state for projects.
package writingstate

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/0verL1nk/GraduateProject/server/internal/auth"
	"github.com/0verL1nk/GraduateProject/server/internal/httpapi"
	"github.com/0verL1nk/GraduateProject/server/internal/store"
)

const maxStateBytes = 256 * 1024

type Brief struct {
	Topic          string   `json:"topic"`
	Background     string   `json:"background"`
	Questions      []string `json:"questions"`
	Goals          []string `json:"goals"`
	ScopeOut       []string `json:"scope_out"`
	ObjectsAndData string   `json:"objects_and_data"`
	Methods        string   `json:"methods"`
	KeyChapters    []string `json:"key_chapters"`
	Milestones     []string `json:"milestones"`
	Constraints    []string `json:"constraints"`
	KnownFacts     []string `json:"known_facts"`
	Assumptions    []string `json:"assumptions"`
	Unverified     []string `json:"unverified"`
}

type Requirement struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Source string `json:"source"`
	Note   string `json:"note"`
}

type State struct {
	ProjectID    uuid.UUID         `json:"project_id"`
	Scenario     string            `json:"scenario"`
	Brief        Brief             `json:"brief"`
	Requirements []Requirement     `json:"requirements"`
	Phases       map[string]string `json:"phases"`
	CurrentPhase string            `json:"current_phase"`
	Revision     int               `json:"revision"`
}

type Service struct{ DB *gorm.DB }

func NewService(db *gorm.DB) *Service { return &Service{DB: db} }

func defaultState(projectID uuid.UUID, scenario string) State {
	return State{
		ProjectID: projectID,
		Scenario:  scenario,
		Brief: Brief{
			Questions: []string{}, Goals: []string{}, ScopeOut: []string{},
			KeyChapters: []string{}, Milestones: []string{}, Constraints: []string{},
			KnownFacts: []string{}, Assumptions: []string{}, Unverified: []string{},
		},
		Requirements: []Requirement{},
		Phases:       map[string]string{"prepare": "in_progress", "write": "not_started", "review": "not_started"},
		CurrentPhase: "prepare",
		Revision:     0,
	}
}

func (s *Service) project(c *gin.Context) (*store.Project, bool) {
	projectID, err := uuid.Parse(c.Param("project_id"))
	if err != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return nil, false
	}
	uid := auth.CurrentUser(c).ID
	var project store.Project
	if err := s.DB.WithContext(c.Request.Context()).Where("id = ? AND owner_id = ?", store.UUID{UUID: projectID}, uid).First(&project).Error; err != nil {
		httpapi.WriteError(c.Writer, httpapi.NotFound("Project"))
		return nil, false
	}
	return &project, true
}

func (s *Service) get(c *gin.Context) {
	project, ok := s.project(c)
	if !ok {
		return
	}
	state := defaultState(project.ID.UUID, project.Scenario)
	var row store.ProjectWritingState
	err := s.DB.WithContext(c.Request.Context()).Where("project_id = ?", project.ID).First(&row).Error
	if err == nil {
		merged, mergeErr := mergeStored(row)
		if mergeErr != nil {
			httpapi.WriteError(c.Writer, mergeErr)
			return
		}
		state = merged
		state.ProjectID = project.ID.UUID
		state.Scenario = project.Scenario
		state.Revision = row.Revision
	} else if err != gorm.ErrRecordNotFound {
		httpapi.WriteError(c.Writer, err)
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(state, ""))
}

func mergeStored(row store.ProjectWritingState) (State, error) {
	defaults, err := json.Marshal(defaultState(uuid.Nil, "undergraduate_thesis"))
	if err != nil {
		return State{}, err
	}
	base := map[string]any{}
	stored := map[string]any{}
	if err := json.Unmarshal(defaults, &base); err != nil {
		return State{}, err
	}
	for key, raw := range map[string]datatypes.JSON{"brief": row.Brief, "requirements": row.Requirements, "phases": row.Phases} {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return State{}, err
		}
		stored[key] = value
	}
	stored["current_phase"] = row.CurrentPhase
	mergeMaps(base, stored)
	encoded, err := json.Marshal(base)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(encoded, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func mergeMaps(base, updates map[string]any) {
	for key, update := range updates {
		if update == nil {
			continue
		}
		baseMap, baseOK := base[key].(map[string]any)
		updateMap, updateOK := update.(map[string]any)
		if baseOK && updateOK {
			mergeMaps(baseMap, updateMap)
			continue
		}
		base[key] = update
	}
}

func (s *Service) put(c *gin.Context) {
	project, ok := s.project(c)
	if !ok {
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxStateBytes+1))
	if err != nil || len(body) > maxStateBytes {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}
	var request State
	if err := json.Unmarshal(body, &request); err != nil || validate(request) != nil {
		httpapi.WriteError(c.Writer, httpapi.ErrValidation)
		return
	}

	briefJSON, _ := json.Marshal(request.Brief)
	requirementsJSON, _ := json.Marshal(request.Requirements)
	phasesJSON, _ := json.Marshal(request.Phases)
	var latest State
	conflict := false
	err = s.DB.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		var row store.ProjectWritingState
		findErr := tx.Where("project_id = ?", project.ID).First(&row).Error
		if findErr == gorm.ErrRecordNotFound {
			if request.Revision != 0 {
				latest = defaultState(project.ID.UUID, project.Scenario)
				conflict = true
				return nil
			}
			row = store.ProjectWritingState{
				ProjectID: project.ID, Brief: datatypes.JSON(briefJSON),
				Requirements: datatypes.JSON(requirementsJSON), Phases: datatypes.JSON(phasesJSON),
				CurrentPhase: request.CurrentPhase, Revision: 1,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
			latest = request
			latest.ProjectID, latest.Scenario, latest.Revision = project.ID.UUID, project.Scenario, 1
			return nil
		}
		if findErr != nil {
			return findErr
		}
		if row.Revision != request.Revision {
			latest, err = mergeStored(row)
			if err != nil {
				return err
			}
			latest.ProjectID, latest.Scenario, latest.Revision = project.ID.UUID, project.Scenario, row.Revision
			conflict = true
			return nil
		}
		result := tx.Model(&store.ProjectWritingState{}).
			Where("project_id = ? AND revision = ?", project.ID, request.Revision).
			Updates(map[string]any{
				"brief": datatypes.JSON(briefJSON), "requirements": datatypes.JSON(requirementsJSON),
				"phases": datatypes.JSON(phasesJSON), "current_phase": request.CurrentPhase,
				"revision": gorm.Expr("revision + 1"), "updated_at": time.Now().UTC(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			if err := tx.Where("project_id = ?", project.ID).First(&row).Error; err != nil {
				return err
			}
			latest, err = mergeStored(row)
			if err != nil {
				return err
			}
			latest.ProjectID, latest.Scenario, latest.Revision = project.ID.UUID, project.Scenario, row.Revision
			conflict = true
			return nil
		}
		latest = request
		latest.ProjectID, latest.Scenario, latest.Revision = project.ID.UUID, project.Scenario, request.Revision+1
		return nil
	})
	if err != nil {
		httpapi.WriteError(c.Writer, err)
		return
	}
	if conflict {
		_ = httpapi.WriteEnvelope(c.Writer, http.StatusConflict, httpapi.Envelope{
			Code: http.StatusConflict, Data: latest, Info: "写作状态已更新，请加载最新版本或基于最新版本覆盖保存",
		})
		return
	}
	_ = httpapi.WriteEnvelope(c.Writer, http.StatusOK, httpapi.Success(latest, ""))
}

func validate(state State) error {
	if state.Revision < 0 || state.Brief.Questions == nil || state.Brief.Goals == nil ||
		state.Brief.ScopeOut == nil || state.Brief.KeyChapters == nil || state.Brief.Milestones == nil ||
		state.Brief.Constraints == nil || state.Brief.KnownFacts == nil || state.Brief.Assumptions == nil ||
		state.Brief.Unverified == nil || state.Requirements == nil || state.Phases == nil {
		return httpapi.ErrValidation
	}
	if state.CurrentPhase != "prepare" && state.CurrentPhase != "write" && state.CurrentPhase != "review" {
		return httpapi.ErrValidation
	}
	if len(state.Phases) != 3 {
		return httpapi.ErrValidation
	}
	for _, phase := range []string{"prepare", "write", "review"} {
		status, exists := state.Phases[phase]
		if !exists || (status != "not_started" && status != "in_progress" && status != "done") {
			return httpapi.ErrValidation
		}
	}
	if len(state.Requirements) > 200 {
		return httpapi.ErrValidation
	}
	values, err := json.Marshal(state)
	if err != nil || len(values) > maxStateBytes {
		return httpapi.ErrValidation
	}
	var value any
	if err := json.Unmarshal(values, &value); err != nil || !validateValue(value) {
		return httpapi.ErrValidation
	}
	return nil
}

func validateValue(value any) bool {
	switch v := value.(type) {
	case string:
		return len([]rune(v)) <= 20_000
	case []any:
		if len(v) > 200 {
			return false
		}
		for _, child := range v {
			if !validateValue(child) {
				return false
			}
		}
	case map[string]any:
		for _, child := range v {
			if !validateValue(child) {
				return false
			}
		}
	}
	return true
}

func Mount(rg *gin.RouterGroup, authMw gin.HandlerFunc, service *Service) {
	group := rg.Group("/projects", authMw)
	group.GET("/:project_id/writing-state", service.get)
	group.PUT("/:project_id/writing-state", service.put)
}

func init() { store.RegisterModels(&store.ProjectWritingState{}) }
