// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

// EXPERIMENTAL - SUBJECT TO CHANGE

package api4

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"
	"time"

	"github.com/mattermost/mattermost-server/mlog"
	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/services/tracing"
)

const (
	MAXIMUM_PLUGIN_FILE_SIZE = 50 * 1024 * 1024
	// INSTALL_PLUGIN_FROM_URL_HTTP_REQUEST_TIMEOUT defines a high timeout for installing plugins
	// from an external URL to avoid slow connections or large plugins from failing to install.
	INSTALL_PLUGIN_FROM_URL_HTTP_REQUEST_TIMEOUT = 60 * time.Minute
)

func (api *API) InitPlugin() {
	mlog.Debug("EXPERIMENTAL: Initializing plugin api")

	api.BaseRoutes.Plugins.Handle("", api.ApiSessionRequired(uploadPlugin)).Methods("POST")
	api.BaseRoutes.Plugins.Handle("", api.ApiSessionRequired(getPlugins)).Methods("GET")
	api.BaseRoutes.Plugin.Handle("", api.ApiSessionRequired(removePlugin)).Methods("DELETE")
	api.BaseRoutes.Plugins.Handle("/install_from_url", api.ApiSessionRequired(installPluginFromUrl)).Methods("POST")

	api.BaseRoutes.Plugins.Handle("/statuses", api.ApiSessionRequired(getPluginStatuses)).Methods("GET")
	api.BaseRoutes.Plugin.Handle("/enable", api.ApiSessionRequired(enablePlugin)).Methods("POST")
	api.BaseRoutes.Plugin.Handle("/disable", api.ApiSessionRequired(disablePlugin)).Methods("POST")

	api.BaseRoutes.Plugins.Handle("/webapp", api.ApiHandler(getWebappPlugins)).Methods("GET")

	api.BaseRoutes.Plugins.Handle("/marketplace", api.ApiSessionRequired(getMarketplacePlugins)).Methods("GET")
}

func uploadPlugin(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:plugin:uploadPlugin")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	defer span.Finish()
	if !*c.App.Config().PluginSettings.Enable || !*c.App.Config().PluginSettings.EnableUploads {
		c.Err = model.NewAppError("uploadPlugin", "app.plugin.upload_disabled.app_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	if err := r.ParseMultipartForm(MAXIMUM_PLUGIN_FILE_SIZE); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	m := r.MultipartForm

	pluginArray, ok := m.File["plugin"]
	if !ok {
		c.Err = model.NewAppError("uploadPlugin", "api.plugin.upload.no_file.app_error", nil, "", http.StatusBadRequest)
		return
	}

	if len(pluginArray) <= 0 {
		c.Err = model.NewAppError("uploadPlugin", "api.plugin.upload.array.app_error", nil, "", http.StatusBadRequest)
		return
	}

	file, err := pluginArray[0].Open()
	if err != nil {
		c.Err = model.NewAppError("uploadPlugin", "api.plugin.upload.file.app_error", nil, "", http.StatusBadRequest)
		return
	}
	defer file.Close()

	force := false
	if len(m.Value["force"]) > 0 && m.Value["force"][0] == "true" {
		force = true
	}
	manifest, unpackErr := c.App.InstallPlugin(file, force)

	if unpackErr != nil {
		c.Err = unpackErr
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(manifest.ToJson()))
}

func installPluginFromUrl(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:plugin:installPluginFromUrl")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	defer span.Finish()
	if !*c.App.Config().PluginSettings.Enable {
		c.Err = model.NewAppError("installPluginFromUrl", "app.plugin.disabled.app_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	downloadUrl := r.URL.Query().Get("plugin_download_url")

	if !model.IsValidHttpUrl(downloadUrl) {
		c.Err = model.NewAppError("installPluginFromUrl", "api.plugin.install.invalid_url.app_error", nil, "", http.StatusBadRequest)
		return
	}

	u, err := url.ParseRequestURI(downloadUrl)
	if err != nil {
		c.Err = model.NewAppError("installPluginFromUrl", "api.plugin.install.invalid_url.app_error", nil, "", http.StatusBadRequest)
		return
	}

	if !*c.App.Config().PluginSettings.AllowInsecureDownloadUrl && u.Scheme != "https" {
		c.Err = model.NewAppError("installPluginFromUrl", "api.plugin.install.insecure_url.app_error", nil, "", http.StatusBadRequest)
		return
	}

	client := c.App.HTTPService.MakeClient(true)
	client.Timeout = INSTALL_PLUGIN_FROM_URL_HTTP_REQUEST_TIMEOUT

	resp, err := client.Get(downloadUrl)
	if err != nil {
		c.Err = model.NewAppError("installPluginFromUrl", "api.plugin.install.download_failed.app_error", nil, err.Error(), http.StatusBadRequest)
		return
	}
	defer resp.Body.Close()

	force := false
	if r.URL.Query().Get("force") == "true" {
		force = true
	}

	fileBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		c.Err = model.NewAppError("installPluginFromUrl", "api.plugin.install.reading_stream_failed.app_error", nil, err.Error(), http.StatusBadRequest)
		return
	}

	manifest, appErr := c.App.InstallPlugin(bytes.NewReader(fileBytes), force)
	if appErr != nil {
		c.Err = appErr
		return
	}
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(manifest.ToJson()))
}

func getPlugins(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:plugin:getPlugins")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	defer span.Finish()
	if !*c.App.Config().PluginSettings.Enable {
		c.Err = model.NewAppError("getPlugins", "app.plugin.disabled.app_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	response, err := c.App.GetPlugins()
	if err != nil {
		c.Err = err
		return
	}

	w.Write([]byte(response.ToJson()))
}

func getPluginStatuses(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:plugin:getPluginStatuses")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	defer span.Finish()
	if !*c.App.Config().PluginSettings.Enable {
		c.Err = model.NewAppError("getPluginStatuses", "app.plugin.disabled.app_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	response, err := c.App.GetClusterPluginStatuses()
	if err != nil {
		c.Err = err
		return
	}

	w.Write([]byte(response.ToJson()))
}

func removePlugin(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:plugin:removePlugin")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	span.SetTag("PluginId", c.Params.PluginId)
	defer span.Finish()
	c.RequirePluginId()
	if c.Err != nil {
		return
	}

	if !*c.App.Config().PluginSettings.Enable {
		c.Err = model.NewAppError("removePlugin", "app.plugin.disabled.app_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	err := c.App.RemovePlugin(c.Params.PluginId)
	if err != nil {
		c.Err = err
		return
	}

	ReturnStatusOK(w)
}

func getWebappPlugins(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:plugin:getWebappPlugins")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	defer span.Finish()
	if !*c.App.Config().PluginSettings.Enable {
		c.Err = model.NewAppError("getWebappPlugins", "app.plugin.disabled.app_error", nil, "", http.StatusNotImplemented)
		return
	}

	manifests, err := c.App.GetActivePluginManifests()
	if err != nil {
		c.Err = err
		return
	}

	clientManifests := []*model.Manifest{}
	for _, m := range manifests {
		if m.HasClient() {
			manifest := m.ClientManifest()

			// There is no reason to expose the SettingsSchema in this API call; it's not used in the webapp.
			manifest.SettingsSchema = nil
			clientManifests = append(clientManifests, manifest)
		}
	}

	w.Write([]byte(model.ManifestListToJson(clientManifests)))
}

func getMarketplacePlugins(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:plugin:getMarketplacePlugins")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	defer span.Finish()
	if !*c.App.Config().PluginSettings.Enable {
		c.Err = model.NewAppError("getMarketplacePlugins", "app.plugin.disabled.app_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !*c.App.Config().PluginSettings.EnableMarketplace {
		c.Err = model.NewAppError("getMarketplacePlugins", "app.plugin.marketplace_disabled.app_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	filter, err := parseMarketplacePluginFilter(r.URL)
	if err != nil {
		c.Err = model.NewAppError("getMarketplacePlugins", "app.plugin.marshal.app_error", nil, err.Error(), http.StatusInternalServerError)
		return
	}

	plugins, appErr := c.App.GetMarketplacePlugins(filter)
	if appErr != nil {
		c.Err = appErr
		return
	}

	json, err := json.Marshal(plugins)
	if err != nil {
		c.Err = model.NewAppError("getMarketplacePlugins", "app.plugin.marshal.app_error", nil, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Write(json)
}

func enablePlugin(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:plugin:enablePlugin")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	span.SetTag("PluginId", c.Params.PluginId)
	defer span.Finish()
	c.RequirePluginId()
	if c.Err != nil {
		return
	}

	if !*c.App.Config().PluginSettings.Enable {
		c.Err = model.NewAppError("activatePlugin", "app.plugin.disabled.app_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	if err := c.App.EnablePlugin(c.Params.PluginId); err != nil {
		c.Err = err
		return
	}

	ReturnStatusOK(w)
}

func disablePlugin(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:plugin:disablePlugin")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	span.SetTag("PluginId", c.Params.PluginId)
	defer span.Finish()
	c.RequirePluginId()
	if c.Err != nil {
		return
	}

	if !*c.App.Config().PluginSettings.Enable {
		c.Err = model.NewAppError("deactivatePlugin", "app.plugin.disabled.app_error", nil, "", http.StatusNotImplemented)
		return
	}

	if !c.App.SessionHasPermissionTo(c.App.Session, model.PERMISSION_MANAGE_SYSTEM) {
		c.SetPermissionError(model.PERMISSION_MANAGE_SYSTEM)
		return
	}

	if err := c.App.DisablePlugin(c.Params.PluginId); err != nil {
		c.Err = err
		return
	}

	ReturnStatusOK(w)
}

func parseMarketplacePluginFilter(u *url.URL) (*model.MarketplacePluginFilter, error) {
	page, err := parseInt(u, "page", 0)
	if err != nil {
		return nil, err
	}

	perPage, err := parseInt(u, "per_page", 100)
	if err != nil {
		return nil, err
	}

	filter := u.Query().Get("filter")
	serverVersion := u.Query().Get("server_version")

	return &model.MarketplacePluginFilter{
		Page:          page,
		PerPage:       perPage,
		Filter:        filter,
		ServerVersion: serverVersion,
	}, nil
}
