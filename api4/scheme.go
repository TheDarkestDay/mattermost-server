// Copyright (c) 2018-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package api4

import (
	"bytes"
	"io/ioutil"
	"net/http"

	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/services/tracing"
)

func (api *API) InitScheme() {
	api.BaseRoutes.Schemes.Handle("", api.ApiSessionRequired(getSchemes)).Methods("GET")
	api.BaseRoutes.Schemes.Handle("", api.ApiSessionRequired(createScheme)).Methods("POST")
	api.BaseRoutes.Schemes.Handle("/{scheme_id:[A-Za-z0-9]+}", api.ApiSessionRequired(deleteScheme)).Methods("DELETE")
	api.BaseRoutes.Schemes.Handle("/{scheme_id:[A-Za-z0-9]+}", api.ApiSessionRequiredTrustRequester(getScheme)).Methods("GET")
	api.BaseRoutes.Schemes.Handle("/{scheme_id:[A-Za-z0-9]+}/patch", api.ApiSessionRequired(patchScheme)).Methods("PUT")
	api.BaseRoutes.Schemes.Handle("/{scheme_id:[A-Za-z0-9]+}/teams", api.ApiSessionRequiredTrustRequester(getTeamsForScheme)).Methods("GET")
	api.BaseRoutes.Schemes.Handle("/{scheme_id:[A-Za-z0-9]+}/channels", api.ApiSessionRequiredTrustRequester(getChannelsForScheme)).Methods("GET")
}

func createScheme(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:scheme:createScheme")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	defer span.Finish()
	scheme := model.SchemeFromJson(r.Body)
	if scheme == nil {
		c.SetInvalidParam("scheme")
		return
	}

	if c.App.License() == nil || !*c.App.License().Features.CustomPermissionsSchemes {
		c.Err = model.NewAppError("Api4.CreateScheme", "api.scheme.create_scheme.license.error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	scheme, err := c.App.CreateScheme(scheme)
	if err != nil {
		c.Err = err
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(scheme.ToJson()))
}

func getScheme(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:scheme:getScheme")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	span.SetTag("SchemeId", c.Params.SchemeId)
	defer span.Finish()
	c.RequireSchemeId()
	if c.Err != nil {
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	scheme, err := c.App.GetScheme(c.Params.SchemeId)
	if err != nil {
		c.Err = err
		return
	}

	w.Write([]byte(scheme.ToJson()))
}

func getSchemes(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:scheme:getSchemes")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	span.SetTag("Scope", c.Params.Scope)
	span.SetTag("Page", c.Params.Page)
	span.SetTag("PerPage", c.Params.PerPage)
	defer span.Finish()
	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	scope := c.Params.Scope
	if scope != "" && scope != model.SCHEME_SCOPE_TEAM && scope != model.SCHEME_SCOPE_CHANNEL {
		c.SetInvalidParam("scope")
		return
	}

	schemes, err := c.App.GetSchemesPage(c.Params.Scope, c.Params.Page, c.Params.PerPage)
	if err != nil {
		c.Err = err
		return
	}

	w.Write([]byte(model.SchemesToJson(schemes)))
}

func getTeamsForScheme(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:scheme:getTeamsForScheme")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	span.SetTag("SchemeId", c.Params.SchemeId)
	span.SetTag("Page", c.Params.Page)
	span.SetTag("PerPage", c.Params.PerPage)
	defer span.Finish()
	c.RequireSchemeId()
	if c.Err != nil {
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	scheme, err := c.App.GetScheme(c.Params.SchemeId)
	if err != nil {
		c.Err = err
		return
	}

	if scheme.Scope != model.SCHEME_SCOPE_TEAM {
		c.Err = model.NewAppError("Api4.GetTeamsForScheme", "api.scheme.get_teams_for_scheme.scope.error", nil, "", http.StatusBadRequest)
		return
	}

	teams, err := c.App.GetTeamsForSchemePage(scheme, c.Params.Page, c.Params.PerPage)
	if err != nil {
		c.Err = err
		return
	}

	w.Write([]byte(model.TeamListToJson(teams)))
}

func getChannelsForScheme(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:scheme:getChannelsForScheme")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	span.SetTag("SchemeId", c.Params.SchemeId)
	span.SetTag("Page", c.Params.Page)
	span.SetTag("PerPage", c.Params.PerPage)
	defer span.Finish()
	c.RequireSchemeId()
	if c.Err != nil {
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	scheme, err := c.App.GetScheme(c.Params.SchemeId)
	if err != nil {
		c.Err = err
		return
	}

	if scheme.Scope != model.SCHEME_SCOPE_CHANNEL {
		c.Err = model.NewAppError("Api4.GetChannelsForScheme", "api.scheme.get_channels_for_scheme.scope.error", nil, "", http.StatusBadRequest)
		return
	}

	channels, err := c.App.GetChannelsForSchemePage(scheme, c.Params.Page, c.Params.PerPage)
	if err != nil {
		c.Err = err
		return
	}

	w.Write([]byte(channels.ToJson()))
}

func patchScheme(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:scheme:patchScheme")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	span.SetTag("SchemeId", c.Params.SchemeId)
	defer span.Finish()
	c.RequireSchemeId()
	if c.Err != nil {
		return
	}

	patch := model.SchemePatchFromJson(r.Body)
	if patch == nil {
		c.SetInvalidParam("scheme")
		return
	}

	if c.App.License() == nil || !*c.App.License().Features.CustomPermissionsSchemes {
		c.Err = model.NewAppError("Api4.PatchScheme", "api.scheme.patch_scheme.license.error", nil, "", http.StatusNotImplemented)
		return
	}

	scheme, err := c.App.GetScheme(c.Params.SchemeId)
	if err != nil {
		c.Err = err
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	scheme, err = c.App.PatchScheme(scheme, patch)
	if err != nil {
		c.Err = err
		return
	}

	c.LogAudit("")
	w.Write([]byte(scheme.ToJson()))
}

func deleteScheme(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:scheme:deleteScheme")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	span.SetTag("SchemeId", c.Params.SchemeId)
	defer span.Finish()
	c.RequireSchemeId()
	if c.Err != nil {
		return
	}

	if c.App.License() == nil || !*c.App.License().Features.CustomPermissionsSchemes {
		c.Err = model.NewAppError("Api4.DeleteScheme", "api.scheme.delete_scheme.license.error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	if _, err := c.App.DeleteScheme(c.Params.SchemeId); err != nil {
		c.Err = err
		return
	}

	ReturnStatusOK(w)
}
