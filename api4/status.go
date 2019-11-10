// Copyright (c) 2017-present Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package api4

import (
	"bytes"
	"io/ioutil"
	"net/http"

	"github.com/mattermost/mattermost-server/model"
	"github.com/mattermost/mattermost-server/services/tracing"
)

func (api *API) InitStatus() {
	api.BaseRoutes.User.Handle("/status", api.ApiSessionRequired(getUserStatus)).Methods("GET")
	api.BaseRoutes.Users.Handle("/status/ids", api.ApiSessionRequired(getUserStatusesByIds)).Methods("POST")
	api.BaseRoutes.User.Handle("/status", api.ApiSessionRequired(updateUserStatus)).Methods("PUT")
}

func getUserStatus(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context,

		// No permission check required
		"api4:status:getUserStatus")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	span.SetTag("UserId", c.Params.UserId)
	defer span.Finish()
	c.RequireUserId()
	if c.Err != nil {
		return
	}

	statusMap, err := c.App.GetUserStatusesByIds([]string{c.Params.UserId})
	if err != nil {
		c.Err = err
		return
	}

	if len(statusMap) == 0 {
		c.Err = model.NewAppError("UserStatus", "api.status.user_not_found.app_error", nil, "", http.StatusNotFound)
		return
	}

	w.Write([]byte(statusMap[0].ToJson()))
}

func getUserStatusesByIds(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:status:getUserStatusesByIds")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	// No permission check required
	defer span.Finish()
	userIds := model.ArrayFromJson(r.Body)

	if len(userIds) == 0 {
		c.SetInvalidParam("user_ids")
		return
	}

	statusMap, err := c.App.GetUserStatusesByIds(userIds)
	if err != nil {
		c.Err = err
		return
	}

	w.Write([]byte(model.StatusListToJson(statusMap)))
}

func updateUserStatus(c *Context, w http.ResponseWriter, r *http.Request) {
	span, ctx := tracing.StartSpanWithParentByContext(c.App.Context, "api4:status:updateUserStatus")
	c.App.Context = ctx
	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = ioutil.ReadAll(r.Body)
		r.Body = ioutil.NopCloser(bytes.NewBuffer(bodyBytes))
		span.SetTag("body", string(bodyBytes))
	}

	span.SetTag("UserId", c.Params.UserId)
	defer span.Finish()
	c.RequireUserId()
	if c.Err != nil {
		return
	}

	status := model.StatusFromJson(r.Body)
	if status == nil {
		c.SetInvalidParam("status")
		return
	}

	// The user being updated in the payload must be the same one as indicated in the URL.
	if status.UserId != c.Params.UserId {
		c.SetInvalidParam("user_id")
		return
	}

	if !c.App.SessionHasPermissionToUser(c.App.Session, c.Params.UserId) {
		c.SetPermissionError(model.PERMISSION_EDIT_OTHER_USERS)
		return
	}

	currentStatus, err := c.App.GetStatus(c.Params.UserId)
	if err == nil && currentStatus.Status == model.STATUS_OUT_OF_OFFICE && status.Status != model.STATUS_OUT_OF_OFFICE {
		c.App.DisableAutoResponder(c.Params.UserId, c.IsSystemAdmin())
	}

	switch status.Status {
	case "online":
		c.App.SetStatusOnline(c.Params.UserId, true)
	case "offline":
		c.App.SetStatusOffline(c.Params.UserId, true)
	case "away":
		c.App.SetStatusAwayIfNeeded(c.Params.UserId, true)
	case "dnd":
		c.App.SetStatusDoNotDisturb(c.Params.UserId)
	default:
		c.SetInvalidParam("status")
		return
	}

	getUserStatus(c, w, r)
}
