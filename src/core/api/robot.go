// Copyright 2018 Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api

import (
	"fmt"
	"github.com/goharbor/harbor/src/common"
	"github.com/goharbor/harbor/src/common/dao"
	"github.com/goharbor/harbor/src/common/models"
	"github.com/goharbor/harbor/src/common/token"
	"net/http"
	"strconv"
)

// RobotAPI ...
type RobotAPI struct {
	BaseController
	project *models.Project
	robot   *models.Robot
}

// Prepare ...
func (r *RobotAPI) Prepare() {
	r.BaseController.Prepare()
	method := r.Ctx.Request.Method

	if !r.SecurityCtx.IsAuthenticated() {
		r.HandleUnauthorized()
		return
	}

	pid, err := r.GetInt64FromPath(":pid")
	if err != nil || pid <= 0 {
		var errMsg string
		if err != nil {
			errMsg = "failed to get project ID " + err.Error()
		} else {
			errMsg = "invalid project ID: " + fmt.Sprintf("%d", pid)
		}
		r.HandleBadRequest(errMsg)
		return
	}
	project, err := r.ProjectMgr.Get(pid)
	if err != nil {
		r.ParseAndHandleError(fmt.Sprintf("failed to get project %d", pid), err)
		return
	}
	if project == nil {
		r.HandleNotFound(fmt.Sprintf("project %d not found", pid))
		return
	}
	r.project = project

	if method == http.MethodPut || method == http.MethodDelete {
		id, err := r.GetInt64FromPath(":id")
		if err != nil || id <= 0 {
			r.HandleBadRequest("invalid robot ID")
			return
		}

		robot, err := dao.GetRobotByID(id)
		if err != nil {
			r.HandleInternalServerError(fmt.Sprintf("failed to get robot %d: %v", id, err))
			return
		}

		if robot == nil {
			r.HandleNotFound(fmt.Sprintf("robot %d not found", id))
			return
		}

		r.robot = robot
	}

	if !(r.Ctx.Input.IsGet() && r.SecurityCtx.HasReadPerm(pid) ||
		r.SecurityCtx.HasAllPerm(pid)) {
		r.HandleForbidden(r.SecurityCtx.GetUsername())
		return
	}

}

// Post ...
func (r *RobotAPI) Post() {
	var robotReq models.RobotReq
	r.DecodeJSONReq(&robotReq)
	createdName := common.RobotPrefix + robotReq.Name

	// first to add a robot account, and get its id.
	robot := models.Robot{
		Name:        createdName,
		Description: robotReq.Description,
		ProjectID:   r.project.ProjectID,
	}
	id, err := dao.AddRobot(&robot)
	if err != nil {
		if err == dao.ErrDupRows {
			r.HandleConflict()
			return
		}
		r.HandleInternalServerError(fmt.Sprintf("failed to create robot account: %v", err))
		return
	}

	// generate the token, and return it with response data.
	// token is not stored in the database.
	jwtToken, err := token.New(id, r.project.ProjectID, robotReq.Access)
	if err != nil {
		r.HandleInternalServerError(fmt.Sprintf("failed to valid parameters to generate token for robot account, %v", err))
		err := dao.DeleteRobot(id)
		if err != nil {
			r.HandleInternalServerError(fmt.Sprintf("failed to delete the robot account: %d, %v", id, err))
		}
		return
	}

	rawTk, err := jwtToken.Raw()
	if err != nil {
		r.HandleInternalServerError(fmt.Sprintf("failed to sign token for robot account, %v", err))
		err := dao.DeleteRobot(id)
		if err != nil {
			r.HandleInternalServerError(fmt.Sprintf("failed to delete the robot account: %d, %v", id, err))
		}
		return
	}

	robotRep := models.RobotRep{
		Name:  robot.Name,
		Token: rawTk,
	}
	r.Redirect(http.StatusCreated, strconv.FormatInt(id, 10))
	r.Data["json"] = robotRep
	r.ServeJSON()
}

// List list all the robots of a project
func (r *RobotAPI) List() {
	query := models.RobotQuery{
		ProjectID: r.project.ProjectID,
	}

	count, err := dao.CountRobot(&query)
	if err != nil {
		r.HandleInternalServerError(fmt.Sprintf("failed to list robots on project: %d, %v", r.project.ProjectID, err))
		return
	}
	query.Page, query.Size = r.GetPaginationParams()

	robots, err := dao.ListRobots(&query)
	if err != nil {
		r.HandleInternalServerError(fmt.Sprintf("failed to get robots %v", err))
		return
	}

	r.SetPaginationHeader(count, query.Page, query.Size)
	r.Data["json"] = robots
	r.ServeJSON()
}

// Get get robot by id
func (r *RobotAPI) Get() {
	id, err := r.GetInt64FromPath(":id")
	if err != nil || id <= 0 {
		r.HandleBadRequest(fmt.Sprintf("invalid robot ID: %s", r.GetStringFromPath(":id")))
		return
	}

	robot, err := dao.GetRobotByID(id)
	if err != nil {
		r.HandleInternalServerError(fmt.Sprintf("failed to get robot %d: %v", id, err))
		return
	}
	if robot == nil {
		r.HandleNotFound(fmt.Sprintf("robot %d not found", id))
		return
	}

	r.Data["json"] = robot
	r.ServeJSON()
}

// Put disable or enable a robot account
func (r *RobotAPI) Put() {
	var robotReq models.RobotReq
	r.DecodeJSONReqAndValidate(&robotReq)
	r.robot.Disabled = robotReq.Disabled

	if err := dao.UpdateRobot(r.robot); err != nil {
		r.HandleInternalServerError(fmt.Sprintf("failed to update robot %d: %v", r.robot.ID, err))
		return
	}

}

// Delete delete robot by id
func (r *RobotAPI) Delete() {
	if err := dao.DeleteRobot(r.robot.ID); err != nil {
		r.HandleInternalServerError(fmt.Sprintf("failed to delete robot %d: %v", r.robot.ID, err))
		return
	}
}
