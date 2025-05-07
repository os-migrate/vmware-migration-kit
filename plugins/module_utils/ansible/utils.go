/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2024 Red Hat, Inc.
 *
 */

package ansible

import (
	"encoding/json"
	"fmt"
	"os"
)

type Response struct {
	Msg     string   `json:"msg"`
	Changed bool     `json:"changed"`
	Failed  bool     `json:"failed"`
	ID      []string `json:"id"`
}

type MigrateResponse struct {
	Msg     string `json:"msg"`
	Changed bool   `json:"changed"`
	Failed  bool   `json:"failed"`
	Disks   []Disk `json:"disks"`
}

type Disk struct {
	ID      string `json:"id"`
	Primary bool   `json:"primary"`
}

func ExitJson(responseBody Response) {
	returnResponse(responseBody)
}

func FailJson(responseBody Response) {
	responseBody.Failed = true
	returnResponse(responseBody)
}

func RequireField(field, errorMessage string) string {
	if field == "" {
		FailWithMessage(errorMessage)
	}
	return field
}

func DefaultIfEmpty(field, defaultValue string) string {
	if field == "" {
		return defaultValue
	}
	return field
}

func FailWithMessage(msg string) {
	response := Response{Msg: msg}
	FailJson(response)
}

func returnResponse(responseBody Response) {
	var response []byte
	var err error
	response, err = json.Marshal(responseBody)
	if err != nil {
		response, _ = json.Marshal(Response{Msg: "Invalid response object"})
	}
	fmt.Println(string(response))
	if responseBody.Failed {
		os.Exit(1)
	} else {
		os.Exit(0)
	}
}

// TODO: commented since it's not used
/*
func returnMResponse(responseBody Response) []Disk {
	var disks []Disk
	for _, id := range responseBody.ID {
		disks = append(disks, Disk{ID: id, Primary: true})
	}
	return disks
}
*/
