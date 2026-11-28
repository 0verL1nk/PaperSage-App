package review_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/0verL1nk/GraduateProject/server/internal/store"
	"github.com/0verL1nk/GraduateProject/server/internal/testutil"
)

// Create 必须把检查单元连同 sources_read_json 一起落库；该列 NOT NULL，
// 漏写会在生产直接 500。
func TestCreateReviewRunPersistsUnits(t *testing.T) {
	ts := testutil.New(t)

	resp := ts.Do(t, http.MethodPost, "/api/v1/projects", []byte(`{"name":"review-create-test"}`))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var project struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&project))
	require.NotEmpty(t, project.Data.ID)

	resp = ts.Do(t, http.MethodGet, "/api/v1/projects/"+project.Data.ID+"/review/options", nil)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	body := []byte(`{"clientRequestId":"review-create-test-1","scope":{"filePaths":["main.tex"],"dimensions":["goal_conclusion"]}}`)
	resp = ts.Do(t, http.MethodPost, "/api/v1/projects/"+project.Data.ID+"/review-runs", body)
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var run struct {
		Data struct {
			ID    string `json:"id"`
			Units []struct {
				ID string `json:"id"`
			} `json:"units"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&run))
	require.NotEmpty(t, run.Data.ID)
	require.NotEmpty(t, run.Data.Units)

	var units []store.ReviewUnit
	require.NoError(t, ts.DB.Order("seq").Find(&units).Error)
	require.NotEmpty(t, units)
	for _, unit := range units {
		require.NotEmpty(t, unit.SourcesReadJSON, "review unit must persist a JSON sources list; the column is NOT NULL")
	}
}
