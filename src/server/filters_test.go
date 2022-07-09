package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/jackc/pgtype"
	"github.com/labstack/echo/v4"
	"github.com/letsblockit/letsblockit/src/db"
	"github.com/letsblockit/letsblockit/src/filters"
	"github.com/letsblockit/letsblockit/src/pages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var filter1 = &filters.Filter{
	Name: "filter1",
	Tags: []string{"tag1", "tag2"},
}

var filter2 = &filters.Filter{
	Name:  "filter2",
	Title: "TITLE2",
	Params: []filters.FilterParam{{
		Name:    "one",
		Type:    filters.StringParam,
		Default: "default",
	}, {
		Name:    "two",
		Type:    filters.BooleanParam,
		Default: true,
	}, {
		Name:    "three",
		Type:    filters.StringListParam,
		Default: []string{"a", "b"},
	}},
	Tags: []string{"tag2", "tag3"},
}

var filter2Defaults = map[string]interface{}{
	"one":   "default",
	"two":   true,
	"three": []string{"a", "b"},
}

var filter3 = &filters.Filter{
	Name: "filter3",
	Tags: []string{"tag3"},
}

func (s *ServerTestSuite) TestListFilters_OK() {
	req := httptest.NewRequest(http.MethodGet, "/filters", nil)
	req.AddCookie(verifiedCookie)

	tList := []string{"tag1", "tag2", "tag3"}
	s.expectF.GetTags().Return(tList)
	s.expectF.GetFilter("filter1").Return(filter1, nil)
	s.expectF.GetFilter("filter2").Return(filter2, nil)
	s.expectF.GetFilters().Return([]*filters.Filter{filter1, filter2, filter3})

	require.NoError(s.T(), s.server.upsertFilterParams(s.c, s.user, "filter1", nil))
	require.NoError(s.T(), s.server.upsertFilterParams(s.c, s.user, "filter2", nil))
	s.markListDownloaded()

	s.expectRender("list-filters", pages.ContextData{
		"filter_tags":       tList,
		"active_filters":    []*filters.Filter{filter1, filter2},
		"available_filters": []*filters.Filter{filter3},
		"list_downloaded":   true,
		"updated_filters":   map[string]bool{"filter2": true},
	})
	s.runRequest(req, assertOk)
}

func (s *ServerTestSuite) TestListFilters_ByTag() {
	req := httptest.NewRequest(http.MethodGet, "/filters/tag/tag2", nil)
	req.AddCookie(verifiedCookie)

	require.NoError(s.T(), s.server.upsertFilterParams(s.c, s.user, "filter1", nil))

	tList := []string{"tag1", "tag2", "tag3"}
	s.expectF.GetTags().Return(tList)
	s.expectF.GetFilters().Return([]*filters.Filter{filter1, filter2, filter3})
	s.expectF.GetFilter("filter1").Return(filter1, nil)

	s.expectRender("list-filters", pages.ContextData{
		"filter_tags":       tList,
		"tag_search":        "tag2",
		"active_filters":    []*filters.Filter{filter1},
		"available_filters": []*filters.Filter{filter2},
		"list_downloaded":   false,
	})
	s.runRequest(req, assertOk)
}

func (s *ServerTestSuite) TestViewFilter_Anonymous() {
	req := httptest.NewRequest(http.MethodGet, "/filters/filter2", nil)
	s.expectF.GetFilter("filter2").Return(filter2, nil)
	s.expectRenderFilter("filter2", filter2Defaults, "output")
	s.expectRender("view-filter", pages.ContextData{
		"filter":   filter2,
		"rendered": "output",
		"params":   filter2Defaults,
	})
	s.runRequest(req, assertOk)
}

func (s *ServerTestSuite) TestViewFilter_NoInstance() {
	req := httptest.NewRequest(http.MethodGet, "/filters/filter2", nil)
	req.AddCookie(verifiedCookie)
	s.expectF.GetFilter("filter2").Return(filter2, nil)
	s.expectRenderFilter("filter2", filter2Defaults, "output")
	s.expectRender("view-filter", pages.ContextData{
		"filter":   filter2,
		"rendered": "output",
		"params":   filter2Defaults,
	})
	s.runRequest(req, assertOk)
}

func (s *ServerTestSuite) TestViewFilter_HasInstance() {
	req := httptest.NewRequest(http.MethodGet, "/filters/filter2", nil)
	req.AddCookie(verifiedCookie)
	s.expectF.GetFilter("filter2").Return(filter2, nil)

	params := map[string]interface{}{"one": "1", "two": false}
	require.NoError(s.T(), s.server.upsertFilterParams(s.c, s.user, "filter2", params))
	s.expectRenderFilter("filter2", params, "output")
	s.expectRender("view-filter", pages.ContextData{
		"filter":       filter2,
		"rendered":     "output",
		"params":       params,
		"has_instance": true,
		"new_params":   map[string]bool{"three": true},
	})
	s.runRequest(req, assertOk)
}

func (s *ServerTestSuite) TestViewFilter_Preview() {
	f := buildFilter2FormBody()
	f.Add(csrfLookup, s.csrf)
	req := httptest.NewRequest(http.MethodPost, "/filters/filter2", strings.NewReader(f.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(verifiedCookie)
	s.expectF.GetFilter("filter2").Return(filter2, nil)
	params := map[string]interface{}{
		"one":   "1",
		"two":   false,
		"three": []string{"option1", "option2"},
	}

	s.expectRenderFilter("filter2", params, "output")
	s.expectRender("view-filter", pages.ContextData{
		"filter":   filter2,
		"params":   params,
		"rendered": "output",
	})
	s.runRequest(req, assertOk)
	s.requireInstanceCount("filter2", 0)
}

func (s *ServerTestSuite) TestViewFilter_Create() {
	f := buildFilter2FormBody()
	f.Add(csrfLookup, s.csrf)
	f.Add("__save", "")
	req := httptest.NewRequest(http.MethodPost, "/filters/filter2", strings.NewReader(f.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(verifiedCookie)
	s.expectF.GetFilter("filter2").Return(filter2, nil)

	params := map[string]interface{}{
		"one":   "1",
		"two":   false,
		"three": []string{"option1", "option2"},
	}
	s.expectRenderFilter("filter2", params, "output")
	s.expectRender("view-filter", pages.ContextData{
		"filter":       filter2,
		"params":       params,
		"rendered":     "output",
		"has_instance": true,
		"saved_ok":     true,
	})
	s.runRequest(req, assertOk)

	stored, err := s.store.GetInstanceForUserAndFilter(context.Background(), db.GetInstanceForUserAndFilterParams{
		UserID:     s.user,
		FilterName: "filter2",
	})
	require.NoError(s.T(), err)
	s.requireJSONEq(params, stored)
	s.requireInstanceCount("filter2", 1)
}

func (s *ServerTestSuite) TestViewFilter_CreateEmptyParams() {
	_, err := s.store.CreateListForUser(context.Background(), s.user)
	require.NoError(s.T(), err)

	f := buildFilter2FormBody() // Add params that will be ignored: filter1 does not have any
	f.Add(csrfLookup, s.csrf)
	f.Add("__save", "")
	req := httptest.NewRequest(http.MethodPost, "/filters/filter1", strings.NewReader(f.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(verifiedCookie)
	s.expectF.GetFilter("filter1").Return(filter1, nil)

	s.expectRenderFilter("filter1", map[string]interface{}{}, "output")
	s.expectRender("view-filter", pages.ContextData{
		"filter":       filter1,
		"params":       map[string]interface{}{},
		"rendered":     "output",
		"has_instance": true,
		"saved_ok":     true,
	})
	s.runRequest(req, assertOk)

	stored, err := s.store.GetInstanceForUserAndFilter(context.Background(), db.GetInstanceForUserAndFilterParams{
		UserID:     s.user,
		FilterName: "filter1",
	})
	require.NoError(s.T(), err)
	require.Equal(s.T(), pgtype.Null, stored.Status)
	s.requireInstanceCount("filter1", 1)
}

func (s *ServerTestSuite) TestViewFilter_Update() {
	require.NoError(s.T(), s.server.upsertFilterParams(s.c, s.user, "filter2", nil))
	s.requireInstanceCount("filter2", 1)

	f := buildFilter2FormBody()
	f.Add(csrfLookup, s.csrf)
	f.Add("__save", "")
	req := httptest.NewRequest(http.MethodPost, "/filters/filter2", strings.NewReader(f.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(verifiedCookie)
	s.expectF.GetFilter("filter2").Return(filter2, nil)

	params := map[string]interface{}{
		"one":   "1",
		"two":   false,
		"three": []string{"option1", "option2"},
	}
	s.expectRenderFilter("filter2", params, "output")
	s.expectRender("view-filter", pages.ContextData{
		"filter":       filter2,
		"params":       params,
		"rendered":     "output",
		"has_instance": true,
		"saved_ok":     true,
	})
	s.runRequest(req, assertOk)

	stored, err := s.store.GetInstanceForUserAndFilter(context.Background(), db.GetInstanceForUserAndFilterParams{
		UserID:     s.user,
		FilterName: "filter2",
	})
	require.NoError(s.T(), err)
	s.requireJSONEq(params, stored)
	s.requireInstanceCount("filter2", 1)

}

func (s *ServerTestSuite) TestViewFilter_Disable() {
	require.NoError(s.T(), s.server.upsertFilterParams(s.c, s.user, "filter2", nil))
	s.requireInstanceCount("filter2", 1)

	f := make(url.Values)
	f.Add(csrfLookup, s.csrf)
	f.Add("__disable", "")
	req := httptest.NewRequest(http.MethodPost, "/filters/filter2", strings.NewReader(f.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(verifiedCookie)
	s.expectF.GetFilter("filter2").Return(filter2, nil)
	s.expectP.RedirectToPage(gomock.Any(), "list-filters")
	s.runRequest(req, assertOk)
	s.requireInstanceCount("filter2", 0)
}

func (s *ServerTestSuite) TestViewFilter_MissingCSRF() {
	f := buildFilter2FormBody()
	f.Add("__save", "")
	req := httptest.NewRequest(http.MethodPost, "/filters/filter2", strings.NewReader(f.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(verifiedCookie)
	s.runRequest(req, func(t *testing.T, recorder *httptest.ResponseRecorder) {
		assert.Equal(t, 400, recorder.Result().StatusCode)
	})
}

func (s *ServerTestSuite) TestViewFilterRender_Defaults() {
	req := httptest.NewRequest(http.MethodPost, "/filters/filter2/render", nil)
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(verifiedCookie)
	s.expectF.GetFilter("filter2").Return(filter2, nil)

	s.expectRenderFilter("filter2", filter2Defaults, "output")
	s.expectRender("view-filter-render", pages.ContextData{
		"rendered": "output",
	})
	s.runRequest(req, assertOk)
}

func (s *ServerTestSuite) TestViewFilterRender_Params() {
	req := httptest.NewRequest(http.MethodPost, "/filters/filter2/render", strings.NewReader(buildFilter2FormBody().Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(verifiedCookie)
	s.expectF.GetFilter("filter2").Return(filter2, nil)

	params := map[string]interface{}{
		"one":   "1",
		"two":   false,
		"three": []string{"option1", "option2"},
	}

	s.expectRenderFilter("filter2", params, "output")
	s.expectRender("view-filter-render", pages.ContextData{
		"rendered": "output",
	})
	s.runRequest(req, assertOk)
}

func (s *ServerTestSuite) TestViewFilterRender_LoggedIn() {
	f := buildFilter2FormBody()
	f.Add("__logged_in", "true")
	req := httptest.NewRequest(http.MethodPost, "/filters/filter2/render", strings.NewReader(f.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	req.AddCookie(verifiedCookie)
	s.expectF.GetFilter("filter2").Return(filter2, nil)

	params := map[string]interface{}{
		"one":   "1",
		"two":   false,
		"three": []string{"option1", "option2"},
	}

	s.expectRenderFilter("filter2", params, "output")
	s.expectRenderWithContext("view-filter-render", &pages.Context{
		NakedContent:    true,
		CurrentSection:  "filters",
		NavigationLinks: navigationLinks,
		UserLoggedIn:    true,
		Data: pages.ContextData{
			"rendered": "output",
		},
	})
	s.runRequest(req, assertOk)
}

func buildFilter2FormBody() url.Values {
	f := make(url.Values)
	f.Add("one", "1")
	f.Add("two", "off")
	f.Add("three", "option1")
	f.Add("three", "option2")
	return f
}

func (s *ServerTestSuite) expectRenderFilter(name string, params interface{}, output string) {
	s.expectF.Render(gomock.Any(), name, params).
		DoAndReturn(func(w io.Writer, _ string, _ map[string]interface{}) error {
			_, err := w.Write([]byte(output))
			return err
		})
}
